#!/usr/bin/env bash
# Сборка телефона: Android APK и, на macOS, iOS IPA.
#
#   API_BASE=http://192.168.1.10:8080 ./prep_app/scripts/build_mobile.sh android
#   API_BASE=http://192.168.1.10:8080 ./prep_app/scripts/build_mobile.sh ios
#   API_BASE=http://192.168.1.10:8080 ./prep_app/scripts/build_mobile.sh all
#
# API_BASE компилируется в приложение. На телефоне 127.0.0.1 — это сам телефон.
#
# Подпись Android, если заданы все значения (иначе debug-ключ):
#   ANDROID_KEYSTORE_BASE64, ANDROID_KEYSTORE_PASSWORD, ANDROID_KEY_ALIAS
#   ANDROID_KEY_PASSWORD — необязательно, иначе берётся пароль хранилища
#
# Подпись iOS, если заданы все три значения (иначе IPA без подписи):
#   IOS_CERTIFICATE_BASE64, IOS_CERTIFICATE_PASSWORD, IOS_PROVISIONING_PROFILE_BASE64
#   IOS_BUNDLE_ID — необязательно, если идентификатор уже записан в профиль
#   IOS_EXPORT_METHOD — auto | development | ad-hoc | app-store | enterprise

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP="${ROOT}/app"
TARGET="${1:-all}"

if [[ -x /opt/flutter/bin/flutter ]]; then
  export PATH="/opt/flutter/bin:${PATH}"
fi

decode_b64() {
  if [[ "$(uname -s)" == "Darwin" ]]; then
    base64 -D
  else
    base64 -d
  fi
}

write_bytes_from_b64() {
  local value="$1"
  local dest="$2"
  printf '%s' "$value" | tr -d '[:space:]' | decode_b64 >"$dest"
  if [[ ! -s "$dest" ]]; then
    echo "Пустой файл после base64: ${dest}" >&2
    exit 1
  fi
}

require_flutter() {
  if ! command -v flutter >/dev/null 2>&1; then
    echo "flutter не найден в PATH. Ожидается SDK 3.47.7, как в prep_app/app/.metadata." >&2
    exit 1
  fi
}

load_version() {
  local version_line default_number
  version_line="$(awk '/^version:/ { print $2; exit }' "${APP}/pubspec.yaml")"
  if [[ -z "$version_line" ]]; then
    echo "В pubspec.yaml нет version." >&2
    exit 1
  fi
  BUILD_NAME="${BUILD_NAME:-${version_line%%+*}}"
  if [[ "$version_line" == *"+"* ]]; then
    default_number="${version_line#*+}"
  else
    default_number=1
  fi
  BUILD_NUMBER="${BUILD_NUMBER:-$default_number}"
}

# Тег v1.2.3 задаёт имя версии. v1.2.3+4 ещё и номер сборки.
apply_release_tag() {
  RELEASE_TAG=""
  if [[ "${GITHUB_REF:-}" != refs/tags/* ]]; then
    return
  fi
  local ref="${GITHUB_REF_NAME:-}"
  if [[ ! "$ref" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)(\+([0-9]+))?$ ]]; then
    echo "Тег релиза должен быть вида v1.2.3 или v1.2.3+4, сейчас: ${ref}" >&2
    exit 1
  fi
  BUILD_NAME="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.${BASH_REMATCH[3]}"
  if [[ -n "${BASH_REMATCH[5]:-}" ]]; then
    BUILD_NUMBER="${BASH_REMATCH[5]}"
  fi
  RELEASE_TAG="$ref"
  echo "Релизный тег ${RELEASE_TAG}: версия ${BUILD_NAME}+${BUILD_NUMBER}"
}

check_api_base() {
  API_BASE="${API_BASE:-http://127.0.0.1:8080}"
  API_BASE="${API_BASE%/}"
  case "$API_BASE" in
    http://*|https://*) ;;
    *)
      echo "API_BASE должен начинаться с http:// или https://, сейчас: ${API_BASE}" >&2
      exit 1
      ;;
  esac
  if [[ "$API_BASE" == *[[:space:]]* ]]; then
    echo "API_BASE не должен содержать пробелы." >&2
    exit 1
  fi
  case "$API_BASE" in
    http://127.0.0.1*|http://localhost*|https://127.0.0.1*|https://localhost*)
      echo "Внимание: API_BASE=${API_BASE}. На телефоне это адрес самого телефона, а не компьютера с сервером." >&2
      echo "Для установки на устройство задайте адрес машины, например API_BASE=http://192.168.1.10:8080." >&2
      ;;
  esac
}

relax_ci_gradle_heap() {
  local props="${APP}/android/gradle.properties"
  if [[ "${CI:-}" != "true" || ! -f "$props" ]]; then
    return
  fi
  # Локальный -Xmx8G не помещается в память GitHub-hosted runner.
  sed -i.bak \
    's/^org.gradle.jvmargs=.*/org.gradle.jvmargs=-Xmx2G -XX:MaxMetaspaceSize=512m -XX:ReservedCodeCacheSize=256m/' \
    "$props"
  rm -f "${props}.bak"
}

prepare_android_signing() {
  ANDROID_SIGNING="debug"
  if [[ -z "${ANDROID_KEYSTORE_BASE64:-}" ]]; then
    echo "Android: свой keystore не задан, release подписывается debug-ключом."
    echo "Такой APK ставится на устройство. Обновление поверх старого APK может не встать, если ключ другой: тогда приложение удаляют и ставят заново."
    return
  fi
  if [[ -z "${ANDROID_KEYSTORE_PASSWORD:-}" || -z "${ANDROID_KEY_ALIAS:-}" ]]; then
    echo "Задан ANDROID_KEYSTORE_BASE64, но нет ANDROID_KEYSTORE_PASSWORD или ANDROID_KEY_ALIAS." >&2
    exit 1
  fi
  local ks
  ks="${RUNNER_TEMP:-/tmp}/prep-app-release.keystore"
  write_bytes_from_b64 "$ANDROID_KEYSTORE_BASE64" "$ks"
  export ANDROID_KEYSTORE_PATH="$ks"
  if [[ -z "${ANDROID_KEY_PASSWORD:-}" ]]; then
    export ANDROID_KEY_PASSWORD="$ANDROID_KEYSTORE_PASSWORD"
  fi
  ANDROID_SIGNING="release"
  echo "Android: подпись release-keystore."
}

first_match() {
  local candidate
  for candidate in "$@"; do
    if [[ -f "$candidate" || -d "$candidate" ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}

run_in_app() {
  (cd "$APP" && "$@")
}

copy_apk() {
  local apk name
  apk="$(first_match "${APP}/build/app/outputs/flutter-apk/"*.apk)" || apk=""
  if [[ -z "$apk" ]]; then
    echo "APK не найден после сборки." >&2
    exit 1
  fi
  name="prep_app-${BUILD_NAME}+${BUILD_NUMBER}.apk"
  cp "$apk" "${OUT}/${name}"
  echo "APK: ${OUT}/${name}"
}

build_android() {
  local info="${OUT}/android-build-info.txt" status
  relax_ci_gradle_heap
  prepare_android_signing
  set +e
  run_in_app flutter pub get
  status=$?
  if [[ "$status" -eq 0 ]]; then
    run_in_app flutter build apk --release \
      --build-name="$BUILD_NAME" \
      --build-number="$BUILD_NUMBER" \
      --dart-define="API_BASE=${API_BASE}"
    status=$?
  fi
  set -e
  if [[ -n "${ANDROID_KEYSTORE_PATH:-}" ]]; then
    rm -f "$ANDROID_KEYSTORE_PATH"
    unset ANDROID_KEYSTORE_PATH
  fi
  if [[ "$status" -ne 0 ]]; then
    exit "$status"
  fi
  copy_apk
  cat >"$info" <<EOF
platform: android
api_base: ${API_BASE}
version: ${BUILD_NAME}+${BUILD_NUMBER}
tag: ${RELEASE_TAG:-}
signing: ${ANDROID_SIGNING}
artifact: ${OUT}/prep_app-${BUILD_NAME}+${BUILD_NUMBER}.apk
EOF
}

ios_supported() {
  [[ "$(uname -s)" == "Darwin" ]]
}

remove_stale_signing() {
  rm -f "${APP}/ios/Flutter/Signing.xcconfig"
}

cleanup_ios_signing() {
  if [[ -n "${PBX_BACKUP:-}" && -f "$PBX_BACKUP" ]]; then
    cp "$PBX_BACKUP" "${APP}/ios/Runner.xcodeproj/project.pbxproj"
    rm -f "$PBX_BACKUP"
  fi
  rm -f "${APP}/ios/Flutter/Signing.xcconfig"
  if [[ -n "${KEYCHAIN_PATH:-}" ]]; then
    security delete-keychain "$KEYCHAIN_PATH" >/dev/null 2>&1 || true
  fi
  rm -f "${CERT_PATH:-}" "${PP_PATH:-}"
}

install_ios_identity() {
  local cert_path pp_path plist_path keychain_path keychain_password identity uuid
  cert_path="${RUNNER_TEMP:-/tmp}/prep-app-ios.p12"
  pp_path="${RUNNER_TEMP:-/tmp}/prep-app-ios.mobileprovision"
  plist_path="${RUNNER_TEMP:-/tmp}/prep-app-ios-profile.plist"
  keychain_path="${RUNNER_TEMP:-/tmp}/prep-app-signing.keychain-db"
  keychain_password="$(openssl rand -base64 32)"
  CERT_PATH="$cert_path"
  PP_PATH="$pp_path"
  KEYCHAIN_PATH="$keychain_path"
  trap cleanup_ios_signing EXIT

  write_bytes_from_b64 "$IOS_CERTIFICATE_BASE64" "$cert_path"
  write_bytes_from_b64 "$IOS_PROVISIONING_PROFILE_BASE64" "$pp_path"

  security create-keychain -p "$keychain_password" "$keychain_path"
  security set-keychain-settings -lut 21600 "$keychain_path"
  security unlock-keychain -p "$keychain_password" "$keychain_path"
  if ! security import "$cert_path" -P "$IOS_CERTIFICATE_PASSWORD" -A -t cert -f pkcs12 -k "$keychain_path"; then
    echo "Не удалось импортировать сертификат. Проверьте IOS_CERTIFICATE_BASE64 и IOS_CERTIFICATE_PASSWORD." >&2
    exit 1
  fi
  security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$keychain_password" "$keychain_path"
  security list-keychains -d user -s "$keychain_path"

  identity=""
  local identity_line
  while IFS= read -r identity_line; do
    if [[ "$identity_line" =~ \"(.*)\" ]]; then
      identity="${BASH_REMATCH[1]}"
      break
    fi
  done < <(security find-identity -v -p codesigning "$keychain_path")
  if [[ -z "$identity" ]]; then
    echo "В сертификате нет identity для подписи кода." >&2
    exit 1
  fi

  if ! security cms -D -i "$pp_path" >"$plist_path"; then
    echo "IOS_PROVISIONING_PROFILE_BASE64 не является provisioning profile." >&2
    exit 1
  fi
  uuid="$(/usr/libexec/PlistBuddy -c 'Print UUID' "$plist_path")"
  PROFILE_NAME="$(/usr/libexec/PlistBuddy -c 'Print Name' "$plist_path")"
  TEAM_ID="$(/usr/libexec/PlistBuddy -c 'Print TeamIdentifier:0' "$plist_path")"
  local app_id bundle_from_profile method
  app_id="$(/usr/libexec/PlistBuddy -c 'Print Entitlements:application-identifier' "$plist_path")"
  bundle_from_profile="${app_id#*.}"

  if [[ "$bundle_from_profile" == "*" ]]; then
    if [[ -z "${IOS_BUNDLE_ID:-}" ]]; then
      echo "Профиль с маской *. Задайте IOS_BUNDLE_ID." >&2
      exit 1
    fi
    IOS_BUNDLE="$IOS_BUNDLE_ID"
  elif [[ -n "${IOS_BUNDLE_ID:-}" && "$IOS_BUNDLE_ID" != "$bundle_from_profile" ]]; then
    echo "IOS_BUNDLE_ID (${IOS_BUNDLE_ID}) не совпадает с профилем (${bundle_from_profile})." >&2
    exit 1
  else
    IOS_BUNDLE="$bundle_from_profile"
  fi
  if [[ ! "$IOS_BUNDLE" =~ ^[A-Za-z0-9.-]+$ ]]; then
    echo "Недопустимый bundle id: ${IOS_BUNDLE}" >&2
    exit 1
  fi

  if /usr/libexec/PlistBuddy -c 'Print ProvisionsAllDevices' "$plist_path" >/dev/null 2>&1; then
    method="enterprise"
  elif /usr/libexec/PlistBuddy -c 'Print ProvisionedDevices' "$plist_path" >/dev/null 2>&1; then
    if /usr/libexec/PlistBuddy -c 'Print Entitlements:get-task-allow' "$plist_path" 2>/dev/null | grep -q true; then
      method="development"
    else
      method="ad-hoc"
    fi
  else
    method="app-store"
  fi
  case "${IOS_EXPORT_METHOD:-auto}" in
    auto|"") IOS_METHOD="$method" ;;
    development|ad-hoc|app-store|enterprise) IOS_METHOD="$IOS_EXPORT_METHOD" ;;
    *)
      echo "Неизвестный IOS_EXPORT_METHOD: ${IOS_EXPORT_METHOD}" >&2
      exit 1
      ;;
  esac

  mkdir -p "${HOME}/Library/MobileDevice/Provisioning Profiles"
  cp "$pp_path" "${HOME}/Library/MobileDevice/Provisioning Profiles/${uuid}.mobileprovision"

  if [[ "$PROFILE_NAME" == *\"* || "$PROFILE_NAME" == *$'\n'* || "$identity" == *\"* || "$identity" == *$'\n'* ]]; then
    echo "Имя профиля или сертификата содержит символ, который нельзя записать в настройки сборки." >&2
    exit 1
  fi

  local pbx count
  pbx="${APP}/ios/Runner.xcodeproj/project.pbxproj"
  PBX_BACKUP="$(mktemp)"
  cp "$pbx" "$PBX_BACKUP"
  sed -i '' \
    "s|PRODUCT_BUNDLE_IDENTIFIER = com\\.example\\.prepApp;|PRODUCT_BUNDLE_IDENTIFIER = ${IOS_BUNDLE};|g" \
    "$pbx"
  count="$(grep -F -c "PRODUCT_BUNDLE_IDENTIFIER = ${IOS_BUNDLE};" "$pbx" || true)"
  if [[ "$count" -lt 1 ]]; then
    echo "Не удалось записать bundle id в project.pbxproj." >&2
    exit 1
  fi

  cat >"${APP}/ios/Flutter/Signing.xcconfig" <<EOF
CODE_SIGN_STYLE = Manual
DEVELOPMENT_TEAM = ${TEAM_ID}
CODE_SIGN_IDENTITY = ${identity}
PROVISIONING_PROFILE_SPECIFIER = ${PROFILE_NAME}
EOF

  local export_plist="${APP}/build/ios/ExportOptions.plist"
  mkdir -p "${APP}/build/ios"
  rm -f "$export_plist"
  /usr/libexec/PlistBuddy -c "Add :method string ${IOS_METHOD}" "$export_plist"
  /usr/libexec/PlistBuddy -c 'Add :signingStyle string manual' "$export_plist"
  /usr/libexec/PlistBuddy -c "Add :teamID string ${TEAM_ID}" "$export_plist"
  /usr/libexec/PlistBuddy -c 'Add :provisioningProfiles dict' "$export_plist"
  /usr/libexec/PlistBuddy -c "Add :provisioningProfiles:${IOS_BUNDLE} string ${PROFILE_NAME}" "$export_plist"
  /usr/libexec/PlistBuddy -c 'Add :stripSwiftSymbols bool true' "$export_plist"
  EXPORT_PLIST="$export_plist"
  IOS_SIGNING="signed"
  echo "iOS: подпись «${PROFILE_NAME}», метод ${IOS_METHOD}, bundle ${IOS_BUNDLE}."
}

package_unsigned_ipa() {
  local app_path stage name
  app_path="$(first_match "${APP}/build/ios/iphoneos/"*.app)" || app_path=""
  if [[ -z "$app_path" ]]; then
    echo "Runner.app не найден после сборки без подписи." >&2
    exit 1
  fi
  stage="$(mktemp -d)"
  mkdir -p "${stage}/Payload"
  cp -R "$app_path" "${stage}/Payload/"
  name="prep_app-${BUILD_NAME}+${BUILD_NUMBER}-unsigned.ipa"
  (cd "$stage" && zip -qry "${OUT}/${name}" Payload)
  rm -rf "$stage"
  echo "IPA без подписи: ${OUT}/${name}"
  echo "На устройство его нельзя поставить, пока нет сертификата и provisioning profile."
}

copy_signed_ipa() {
  local ipa name
  ipa="$(first_match "${APP}/build/ios/ipa/"*.ipa)" || ipa=""
  if [[ -z "$ipa" ]]; then
    echo "Подписанный IPA не найден." >&2
    exit 1
  fi
  name="prep_app-${BUILD_NAME}+${BUILD_NUMBER}.ipa"
  cp "$ipa" "${OUT}/${name}"
  echo "IPA: ${OUT}/${name}"
}

ios_signing_requested() {
  [[ -n "${IOS_CERTIFICATE_BASE64:-}${IOS_CERTIFICATE_PASSWORD:-}${IOS_PROVISIONING_PROFILE_BASE64:-}" ]]
}

build_ios() {
  local info="${OUT}/ios-build-info.txt" artifact
  if ! ios_supported; then
    echo "Сборка iOS нужна на macOS: flutter build ipa вызывает Xcode." >&2
    exit 1
  fi
  remove_stale_signing
  IOS_SIGNING="unsigned"
  IOS_BUNDLE="${IOS_BUNDLE_ID:-com.example.prepApp}"
  IOS_METHOD="none"
  run_in_app flutter pub get
  if ios_signing_requested; then
    if [[ -z "${IOS_CERTIFICATE_BASE64:-}" || -z "${IOS_CERTIFICATE_PASSWORD:-}" || -z "${IOS_PROVISIONING_PROFILE_BASE64:-}" ]]; then
      echo "Для подписи iOS нужны все три значения: IOS_CERTIFICATE_BASE64, IOS_CERTIFICATE_PASSWORD, IOS_PROVISIONING_PROFILE_BASE64." >&2
      exit 1
    fi
    install_ios_identity
    run_in_app flutter build ipa --release \
      --build-name="$BUILD_NAME" \
      --build-number="$BUILD_NUMBER" \
      --dart-define="API_BASE=${API_BASE}" \
      --export-options-plist="$EXPORT_PLIST"
    copy_signed_ipa
    artifact="prep_app-${BUILD_NAME}+${BUILD_NUMBER}.ipa"
  else
    echo "iOS: сертификат не задан, собирается IPA без подписи."
    run_in_app flutter build ios --release --no-codesign \
      --build-name="$BUILD_NAME" \
      --build-number="$BUILD_NUMBER" \
      --dart-define="API_BASE=${API_BASE}"
    package_unsigned_ipa
    artifact="prep_app-${BUILD_NAME}+${BUILD_NUMBER}-unsigned.ipa"
  fi
  cat >"$info" <<EOF
platform: ios
api_base: ${API_BASE}
version: ${BUILD_NAME}+${BUILD_NUMBER}
tag: ${RELEASE_TAG:-}
signing: ${IOS_SIGNING}
bundle_id: ${IOS_BUNDLE}
export_method: ${IOS_METHOD}
artifact: ${OUT}/${artifact}
EOF
}

usage() {
  echo "Использование: $0 android|ios|all" >&2
  exit 2
}

main() {
  require_flutter
  check_api_base
  load_version
  apply_release_tag
  OUT="${APP}/build/mobile-out"
  mkdir -p "$OUT"
  local flutter_line="" version_line
  while IFS= read -r version_line; do
    if [[ -z "$flutter_line" ]]; then
      flutter_line="$version_line"
    fi
  done < <(flutter --version)
  echo "Flutter: ${flutter_line}"
  echo "Сборка ${TARGET}, ${BUILD_NAME}+${BUILD_NUMBER}, API_BASE=${API_BASE}"

  case "$TARGET" in
    android) build_android ;;
    ios) build_ios ;;
    all)
      build_android
      if ios_supported; then
        build_ios
      else
        echo "iOS пропущен: эта машина не macOS. На GitHub Actions IPA собирает отдельная job."
      fi
      ;;
    *) usage ;;
  esac
}

main
