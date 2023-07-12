package ver

// Struct from: https://cs.opensource.google/go/x/tools/+/master:blog/atom/atom.go
// See also: https://gist.github.com/humorless/732371c7cdd3cf2478973ff76219b894

import (
	"encoding/xml"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	"golang.org/x/tools/blog/atom"
)

// Check called as goroutine
func Check(log logr.Logger, repo, version string) {
	_, _ = IsCheckOk(log, repo, version)
}

// IsCheckOk does version check at git repo and returns result
func IsCheckOk(log logr.Logger, repo, version string) (bool, error) {

	url := strings.TrimSuffix(repo, ".git")
	if !strings.HasPrefix(url, "https://") {
		url = strings.Replace(url, ":", "/", 1)
		url = strings.Replace(url, "git@", "https://", 1)
	}
	log.V(2).Info("Check", "url", url)
	feed := atom.Feed{}
	if resp, err := http.Get(url + "/releases.atom"); err != nil {
		log.V(1).Info("Fetch error", "error", err)
		return false, err
	} else {
		defer resp.Body.Close()
		if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
			log.V(1).Info("Decode error", "error", err)
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
	log.V(0).Info("App version is outdated", "appVersion", version, "sourceVersion", item.Title, "sourceUpdated", item.Updated, "sourceLink", link)
	return false, nil
}
