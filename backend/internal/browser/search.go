package browser

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const defaultSearchURL = "https://www.google.com/search?q={searchTerms}"

var (
	// Chromium templates mark optional parameters as {name?}; they are
	// dropped. Any other placeholder besides {searchTerms} (for example
	// {google:baseURL}) cannot be expanded here.
	optionalSearchParamRe = regexp.MustCompile(`\{[^{}]*\?\}`)
	searchParamRe         = regexp.MustCompile(`\{[^{}]*\}`)
)

// parseSearchTemplate validates an http(s) search template containing
// {searchTerms} (or %s, accepted for hand-written settings) and returns it in
// the {searchTerms} form.
func parseSearchTemplate(template string) (string, bool) {
	template = strings.TrimSpace(template)
	if strings.Contains(template, "%s") && !strings.Contains(template, "{searchTerms}") {
		template = strings.Replace(template, "%s", "{searchTerms}", 1)
	}
	template = optionalSearchParamRe.ReplaceAllString(template, "")
	if strings.Count(template, "{searchTerms}") != 1 ||
		len(searchParamRe.FindAllString(template, -1)) != 1 {
		return "", false
	}
	u, err := url.Parse(strings.Replace(template, "{searchTerms}", "x", 1))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	return template, true
}

// chromiumSearchTemplate reads the default search engine the user picked in
// Chromium's own settings. Chromium mirrors that choice into the profile's
// Preferences file; an untouched profile has no entry (Google).
func chromiumSearchTemplate(profile string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(profile, "Default", "Preferences"))
	if err != nil {
		return "", false
	}
	var prefs struct {
		Provider struct {
			Data struct {
				URL        string `json:"url"`
				PostParams string `json:"post_params"`
			} `json:"template_url_data"`
		} `json:"default_search_provider_data"`
	}
	if json.Unmarshal(data, &prefs) != nil || prefs.Provider.Data.PostParams != "" {
		return "", false
	}
	return parseSearchTemplate(prefs.Provider.Data.URL)
}

// searchTemplate picks the host setting, then Chromium's own choice, then
// Google. It is read per search so a change in Browser setup applies at once.
func (b *Controller) searchTemplate() string {
	if t, ok := parseSearchTemplate(b.cfg.SearchURL); ok {
		return t
	}
	if t, ok := chromiumSearchTemplate(b.cfg.Profile); ok {
		return t
	}
	return defaultSearchURL
}

func searchURL(template, query string) string {
	return strings.Replace(template, "{searchTerms}", url.QueryEscape(query), 1)
}
