# Proje kökünden: source scripts/env.sh
export PATH="$HOME/.local/share/odev-tools/go/bin:$HOME/.local/share/odev-tools:$PATH"
if [ -z "${CHROME_PATH:-}" ]; then
    for chrome_candidate in google-chrome-stable google-chrome chromium chromium-browser; do
        if command -v "$chrome_candidate" >/dev/null 2>&1; then
            export CHROME_PATH="$(command -v "$chrome_candidate")"
            break
        fi
    done
    unset chrome_candidate
fi
