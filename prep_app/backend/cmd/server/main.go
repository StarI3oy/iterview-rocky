package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"prepapp/internal/ai"
	"prepapp/internal/auth"
	"prepapp/internal/httpapi"
	"prepapp/internal/store"
)

func main() {
	addr := flag.String("addr", env("ADDR", ":8080"), "listen address")
	interviewDB := flag.String("interview-db", env("INTERVIEW_DB", "/home/shaman/interview_questions.db"), "path to interview sqlite")
	hardDB := flag.String("hard-db", env("HARD_CONCEPTS_DB", "/home/shaman/hard_concepts.db"), "path to hard concepts sqlite")
	webDir := flag.String("web", env("WEB_DIR", ""), "Flutter web build directory")
	authFile := flag.String("auth-file", env("PREP_AUTH_FILE", ""), "argon2id password file, separate from the databases")
	flag.Parse()

	absInterview, _ := filepath.Abs(*interviewDB)
	absHard, _ := filepath.Abs(*hardDB)

	st, err := store.Open(*interviewDB, *hardDB)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer st.Close()

	authPath := *authFile
	if authPath == "" {
		authPath = filepath.Join(filepath.Dir(absInterview), "prep_auth.json")
	}
	absAuth, _ := filepath.Abs(authPath)
	guard, err := auth.Open(absAuth, auth.RecommendedParams())
	if err != nil {
		log.Fatalf("auth: %v", err)
	}
	message, err := auth.ApplyEnv(guard, os.Getenv("PREP_PASSWORD"), os.Getenv("PREP_PASSWORD_RESET") == "1")
	if err != nil {
		log.Fatalf("auth: %v", err)
	}
	log.Printf("auth file: %s", absAuth)
	log.Printf("%s", message)

	certFile := os.Getenv("PREP_TLS_CERT")
	keyFile := os.Getenv("PREP_TLS_KEY")
	if err := checkTLS(certFile, keyFile); err != nil {
		log.Fatalf("tls: %v", err)
	}

	if knowledge := knowledgePath(); knowledge != "" {
		n, err := st.ImportKnowledge(knowledge)
		if err != nil {
			log.Fatalf("import %s: %v", knowledge, err)
		}
		log.Printf("база знаний: %s (%d записей)", knowledge, n)
	}

	n, err := st.CountItems()
	if err != nil {
		log.Fatalf("count: %v", err)
	}
	if n == 0 {
		log.Printf("prep_items пуст — карточки появятся после загрузки базы (%s)", absInterview)
	} else {
		log.Printf("prep_items: %d", n)
	}

	web := *webDir
	if web != "" {
		if st, err := os.Stat(filepath.Join(web, "index.html")); err != nil || st.IsDir() {
			log.Printf("web dir %s без index.html — раздаю только API", web)
			web = ""
		}
	}

	var completer httpapi.Completer
	if c := ai.New(os.Getenv("OPENROUTER_API_KEY"), os.Getenv("OPENROUTER_MODEL")); c != nil {
		completer = c
	} else {
		log.Printf("OPENROUTER_API_KEY не задан — ревью и пояснение поиска недоступны, поиск по базе работает")
	}

	trustProxy := os.Getenv("PREP_TRUST_PROXY") == "1"
	srv := &http.Server{
		Addr: *addr,
		Handler: (&httpapi.Server{
			Store:          st,
			Completer:      completer,
			WebDir:         web,
			Guard:          guard,
			AllowedOrigins: splitCSV(os.Getenv("PREP_ALLOWED_ORIGINS")),
			TrustProxy:     trustProxy,
		}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		log.Printf("interview db: %s", absInterview)
		log.Printf("hard concepts db: %s", absHard)
		var serveErr error
		if certFile != "" {
			log.Printf("listening with TLS on %s", *addr)
			serveErr = srv.ListenAndServeTLS(certFile, keyFile)
		} else {
			log.Printf("listening on %s", *addr)
			log.Printf("TLS выключен: пароль по сети идёт открытым текстом. Для шифрования задайте PREP_TLS_CERT и PREP_TLS_KEY")
			serveErr = srv.ListenAndServe()
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Fatalf("serve: %v", serveErr)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func knowledgePath() string {
	if p := os.Getenv("KNOWLEDGE_JSON"); p != "" {
		return p
	}
	candidates := []string{
		"/home/shaman/prep_app_knowledge_export.json",
		"prep_app/data/prep_app_knowledge_export.json",
		"data/prep_app_knowledge_export.json",
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "prep_app_knowledge_export.json"),
			filepath.Join(dir, "..", "data", "prep_app_knowledge_export.json"),
		)
	}
	for _, p := range candidates {
		info, err := os.Stat(p)
		if err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func checkTLS(certFile, keyFile string) error {
	if certFile == "" && keyFile == "" {
		return nil
	}
	if certFile == "" || keyFile == "" {
		return errors.New("нужны оба файла PREP_TLS_CERT и PREP_TLS_KEY. Сервер не запущен, базы и ключи не изменены")
	}
	if err := realFile(certFile); err != nil {
		return fmt.Errorf("PREP_TLS_CERT: %w. Сервер не запущен, базы и ключи не изменены", err)
	}
	if err := realFile(keyFile); err != nil {
		return fmt.Errorf("PREP_TLS_KEY: %w. Сервер не запущен, базы и ключи не изменены", err)
	}
	return nil
}

func realFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("это каталог, нужен файл")
	}
	return nil
}
