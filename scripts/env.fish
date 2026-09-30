# Proje kökünden fish içinde: source scripts/env.fish
# Yalnızca bu oturumun PATH değerini değiştirir; tekrar source etmek çoğaltmaz.
for tool_dir in "$HOME/.local/share/odev-tools" "$HOME/.local/share/odev-tools/go/bin"
    if not contains -- "$tool_dir" $PATH
        set -gx PATH "$tool_dir" $PATH
    end
end
if not set -q CHROME_PATH; or test -z "$CHROME_PATH"
    for chrome_candidate in google-chrome-stable google-chrome chromium chromium-browser
        if command -q $chrome_candidate
            set -gx CHROME_PATH (command -s $chrome_candidate)
            break
        end
    end
end
