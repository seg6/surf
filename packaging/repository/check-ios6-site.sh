#!/usr/bin/env bash
# Checks a built site directory against the iOS 6 WebKit profile. Used by
# packaging/repository/verify.sh on the published repository and by CI on
# site/dist, so both enforce the same rules.
set -euo pipefail

site_dir="${1:?usage: check-ios6-site.sh SITE_DIR}"

# Pages may carry small inline ES5 scripts but never load external ones. CSS may
# use flex, grid and gap only inside @supports blocks, which iOS 6 skips, so the
# table and float baseline is what old devices get. Custom properties and
# @font-face are rejected outright.
while IFS= read -r -d '' page; do
  if grep -Eqi '<script[^>]*[[:space:]]src=' "$page"; then
    echo "${page#"$site_dir"/} loads an external script" >&2
    exit 1
  fi
  # Pages are often minified onto one line, so read the whole file and check
  # only the text between <script> and </script>.
  verdict="$(awk '
    BEGIN { RS = "\001" }
    {
      rest = $0
      while ((start = index(tolower(rest), "<script")) > 0) {
        rest = substr(rest, start)
        open_end = index(rest, ">")
        stop = index(tolower(rest), "</script>")
        if (open_end == 0 || stop == 0) break
        body = substr(rest, open_end + 1, stop - open_end - 1)
        if (body ~ /=>|`|(^|[^A-Za-z0-9_$.])(let|const|class)[ \t\r\n]/) bad = 1
        rest = substr(rest, stop + 9)
      }
    }
    END { print bad ? "bad" : "ok" }
  ' "$page")"
  if [[ "$verdict" != ok ]]; then
    echo "${page#"$site_dir"/} has an inline script that is not ES5" >&2
    exit 1
  fi
done < <(find "$site_dir" -type f -name '*.html' -print0)

while IFS= read -r -d '' css; do
  if grep -Eqi 'var\(|@font-face' "$css"; then
    echo "${css#"$site_dir"/} uses a feature outside the iOS 6 compatibility profile" >&2
    exit 1
  fi
  # Scan declarations (minified or not) and track brace depth, so flex, grid
  # and gap are only accepted inside @supports blocks.
  if ! awk '
    function supported(   k) { for (k = 1; k <= depth; k++) if (sup[k]) return 1; return 0 }
    function check(decl) {
      decl = tolower(decl)
      gsub(/[ \t\r\n]+/, "", decl)
      if (!supported() && decl ~ /^(display:(-webkit-)?(inline-)?(flex|grid)|(row-|column-|grid-)?gap:)/) {
        print FILENAME ": " decl > "/dev/stderr"
        bad = 1
      }
    }
    BEGIN { RS = "\001" }
    {
      text = $0
      gsub(/\/\*([^*]|\*+[^*\/])*\*+\//, "", text)
      n = length(text)
      buf = ""
      for (i = 1; i <= n; i++) {
        c = substr(text, i, 1)
        if (c == "{") {
          depth++
          sup[depth] = (tolower(buf) ~ /@supports/) ? 1 : 0
          buf = ""
        } else if (c == "}") {
          check(buf)
          sup[depth] = 0
          depth--
          buf = ""
        } else if (c == ";") {
          check(buf)
          buf = ""
        } else {
          buf = buf c
        }
      }
    }
    END { exit bad }
  ' "$css"; then
    echo "${css#"$site_dir"/} uses flex, grid or gap outside @supports" >&2
    exit 1
  fi
done < <(find "$site_dir" -type f -name '*.css' -print0)
