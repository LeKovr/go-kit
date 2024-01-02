package ver

// Struct from: https://cs.opensource.google/go/x/tools/+/master:blog/atom/atom.go
// See also: https://gist.github.com/humorless/732371c7cdd3cf2478973ff76219b894

import (
	"encoding/xml"
	"log/slog"
	"net/http"
	"strings"

	"github.com/LeKovr/go-kit/slogger"
	"golang.org/x/tools/blog/atom"
)

// Check called as goroutine
func Check(repo, version string) {
	_, _ = IsCheckOk(repo, version)
}

// IsCheckOk does version check at git repo and returns result
func IsCheckOk(repo, version string) (bool, error) {

	url := strings.TrimSuffix(repo, ".git")
	if !strings.HasPrefix(url, "https://") {
		url = strings.Replace(url, ":", "/", 1)
		url = strings.Replace(url, "git@", "https://", 1)
	}
	slog.Debug("Check", "url", url)
	feed := atom.Feed{}
	if resp, err := http.Get(url + "/releases.atom"); err != nil {
		slog.Warn("Fetch error", slogger.ErrAttr(err))
		return false, err
	} else {
		defer resp.Body.Close()
		if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
			slog.Warn("Decode error", slogger.ErrAttr(err))
			return false, err
		}
	}
	if len(feed.Entry) == 0 {
		// repo has no any tags => control disabled, code is actual
		return true, nil
	}
	item := feed.Entry[0]
	if version == item.Title {
		// version is equal, ok
		return true, nil
	}
	var link string
	if len(item.Link) > 0 {
		link = " See " + item.Link[0].Href
	}
	slog.Info("App version is outdated", "appVersion", version, "sourceVersion", item.Title, "sourceUpdated", item.Updated, "sourceLink", link)
	return false, nil
}
