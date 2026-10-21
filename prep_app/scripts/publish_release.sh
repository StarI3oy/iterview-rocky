#!/usr/bin/env bash
# Публикует GitHub Release для тега, которым запущен Actions.
# Файлы лежат в каталоге из первого аргумента (по умолчанию dist).

set -euo pipefail

DIST="${1:-dist}"
tag="${GITHUB_REF_NAME:-}"

if [[ "${GITHUB_REF:-}" != refs/tags/v* ]]; then
  echo "Релиз публикуется только с тега v*. Сейчас: ${GITHUB_REF:-пусто}" >&2
  exit 1
fi
if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(\+[0-9]+)?$ ]]; then
  echo "Тег релиза должен быть вида v1.2.3 или v1.2.3+4, сейчас: ${tag}" >&2
  exit 1
fi
if ! command -v gh >/dev/null 2>&1; then
  echo "Нужна GitHub CLI (gh)." >&2
  exit 1
fi

shopt -s nullglob
files=("${DIST}"/*.apk "${DIST}"/*.ipa "${DIST}"/*build-info.txt)
shopt -u nullglob
if [[ ${#files[@]} -eq 0 ]]; then
  echo "В ${DIST} нет APK, IPA или описания сборки." >&2
  exit 1
fi

notes="$(mktemp)"
{
  echo "Телефонная сборка ${tag}."
  echo
  if [[ -f "${DIST}/android-build-info.txt" ]]; then
    echo '```text'
    cat "${DIST}/android-build-info.txt"
    echo '```'
    echo
  fi
  if [[ -f "${DIST}/ios-build-info.txt" ]]; then
    echo '```text'
    cat "${DIST}/ios-build-info.txt"
    echo '```'
    echo
  fi
  if [[ -f "${DIST}/android-build-info.txt" ]] && grep -q 'signing: debug' "${DIST}/android-build-info.txt"; then
    echo "APK подписан debug-ключом раннера. Постоянная подпись — секреты keystore в Actions."
    echo
  fi
  if [[ -f "${DIST}/ios-build-info.txt" ]] && grep -q 'signing: unsigned' "${DIST}/ios-build-info.txt"; then
    echo "IPA собран без сертификата Apple и на iPhone не ставится."
    echo
  fi
} >"$notes"

if gh release view "$tag" >/dev/null 2>&1; then
  gh release upload "$tag" "${files[@]}" --clobber
  gh release edit "$tag" --notes-file "$notes" --title "$tag"
  echo "Релиз ${tag} обновлён."
else
  gh release create "$tag" "${files[@]}" \
    --verify-tag \
    --title "$tag" \
    --notes-file "$notes"
  echo "Релиз ${tag} создан."
fi
