package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"prepapp/internal/ai"
	"prepapp/internal/httpapi"
	"prepapp/internal/store"
)

func main() {
	addr := flag.String("addr", env("ADDR", ":8080"), "listen address")
	interviewDB := flag.String("interview-db", env("INTERVIEW_DB", "/home/shaman/interview_questions.db"), "path to interview sqlite")
	hardDB := flag.String("hard-db", env("HARD_CONCEPTS_DB", "/home/shaman/hard_concepts.db"), "path to hard concepts sqlite")
	webDir := flag.String("web", env("WEB_DIR", ""), "Flutter web build directory")
	flag.Parse()

	absInterview, _ := filepath.Abs(*interviewDB)
	absHard, _ := filepath.Abs(*hardDB)

	st, err := store.Open(*interviewDB, *hardDB)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer st.Close()

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

	srv := &http.Server{
		Addr:              *addr,
		Handler:           (&httpapi.Server{Store: st, Completer: completer, WebDir: web}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", *addr)
		log.Printf("interview db: %s", absInterview)
		log.Printf("hard concepts db: %s", absHard)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
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
