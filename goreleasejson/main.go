package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/blang/semver"
)

var (
	genDir = flag.String("d", "api", "directory to generate the JSON and text files in")
)

func main() {
	res, err := http.Get("https://golang.org/dl/")
	if err != nil {
		log.Fatalf("goreleasejson: Get /dl: %s", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		log.Fatalf("goreleasejson: status code error: %d %s", res.StatusCode, res.Status)
	}
	doc, err := goquery.NewDocumentFromReader(
		bufio.NewReaderSize(res.Body, 1024*1024),
	)
	if err != nil {
		log.Fatal("goreleasejson: uanble to goquery NewDocumentFromReader: %s", err)
	}
	versions := make(map[string]versInfo)
	doc.Find("div.toggleVisible").Each(func(i int, s *goquery.Selection) {
		addGoVersion(versions, s)
		if err != nil {
			log.Fatalf("goreleasejson: %s", err)
		}
	})
	doc.Find("div.toggle").Each(func(i int, s *goquery.Selection) {
		err := addGoVersion(versions, s)
		if err != nil {
			log.Fatalf("goreleasejson: %s", err)
		}
	})
	artifacts := make(map[string][]artifact)
	// Inefficient but makes it easy to group by versions
	for _, vers := range versions {
		cssEscaped := strings.Replace(vers.rawID, ".", "\\.", -1)
		selector := fmt.Sprintf("div#%s table.codetable tr", cssEscaped)
		doc.Find(selector).Each(func(i int, tr *goquery.Selection) {
			class, found := tr.Attr("class")
			if found && class == "first" {
				// skip their blank first one
				return
			}
			arc := artifact{Version: vers.rawID}
			tds := tr.ChildrenFiltered("td")
			tds.Find("a.download").Each(func(i int, anchor *goquery.Selection) {
				link, found := anchor.Attr("href")
				if !found {
					log.Fatalf("goreleasejson: unable to grab href attribute we think is a download link: %s", err)
				}
				arc.Link = link
			})
			tds = tds.Next()
			arc.Kind = tds.Text()
			tds = tds.Next()
			arc.OS = tds.Text()
			tds = tds.Next()
			arc.Arch = tds.Text()
			tds = tds.Next()
			arc.Size = tds.Text()
			tds = tds.Next()
			arc.SHA256 = tds.Text()
			artifacts[vers.rawID] = append(artifacts[vers.rawID], arc)
		})
	}
	var sortedVersInfo []versInfo
	for _, vers := range versions {
		sortedVersInfo = append(sortedVersInfo, vers)
	}
	sort.Slice(sortedVersInfo, func(i, j int) bool {
		return sortedVersInfo[i].vers.GT(sortedVersInfo[j].vers)
	})
	sortedVers := make([]string, 0, len(sortedVersInfo))
	sortedVersLinks := make([]versionLink, 0, len(sortedVersInfo))
	for _, vers := range sortedVersInfo {
		sortedVers = append(sortedVers, vers.rawID)
		sortedVersLinks = append(sortedVersLinks, versionLink{
			Version: vers.rawID,
			Link:    fmt.Sprintf("/%s/versions/%s/artifacts.json", *genDir, vers.rawID),
		})
	}
	latestVersion := sortedVers[0]

	b, err := json.Marshal(artifacts)
	if err != nil {
		log.Fatalf("goreleasejson: unable to JSON marshal: %s", err)
	}

	err = os.MkdirAll(*genDir, 0774)
	if err != nil {
		log.Fatalf("goreleasejson: unable to create output directory %#v: %s", *genDir, err)
	}
	err = ioutil.WriteFile(filepath.Join(*genDir, "latest_version.txt"), []byte(latestVersion), 0644)
	if err != nil {
		log.Fatalf("goreleasejson: unable to create latest_version.txt file: %s", err)
	}
	err = ioutil.WriteFile(filepath.Join(*genDir, "all_artifacts.json"), []byte(b), 0644)
	for vers, arcs := range artifacts {
		versDir := filepath.Join(*genDir, "versions", vers)
		fp := filepath.Join(versDir, "artifacts.json")
		arcJSON, err := json.Marshal(artifactsWrapper{Artifacts: arcs})
		if err != nil {
			log.Fatalf("goreleasejson: unable to JSON marshal artifacts for %#v: %s", fp, err)
		}
		err = os.MkdirAll(versDir, 0774)
		if err != nil {
			log.Fatalf("goreleasejson: unable to mkdir output directory %#v: %s", versDir, err)
		}
		err = ioutil.WriteFile(
			fp,
			arcJSON,
			0644,
		)
		if err != nil {
			log.Fatalf("goreleasejson: unable write artifacts.json for %#v: %s", fp, err)
		}
	}

	allVersTxtPath := filepath.Join(*genDir, "all_versions.txt")
	versTxtBytes := []byte(strings.Join(sortedVers, "\n"))
	err = ioutil.WriteFile(
		allVersTxtPath,
		versTxtBytes,
		0644,
	)
	if err != nil {
		log.Fatalf("goreleasejson: unable to write %#v: %s", allVersTxtPath, err)
	}

}

func addGoVersion(versions map[string]versInfo, s *goquery.Selection) error {
	id, exists := s.Attr("id")
	if exists && strings.HasPrefix(id, "go") {
		vers, err := semver.ParseTolerant(id[len("go"):])
		if err != nil {
			fmt.Errorf("unable to parse HTML tag's id as a Go version: %w", err)
		}
		versions[id] = versInfo{rawID: id, vers: vers}
	}
	return nil
}

type versInfo struct {
	rawID string
	vers  semver.Version
}

type artifact struct {
	Version string
	Link    string
	Kind    string
	OS      string
	Arch    string
	Size    string
	SHA256  string
}

type artifactsWrapper struct {
	Artifacts []artifact
}

type allVersWrapper struct {
	Versions []versionLink
}

type versionLink struct {
	Version string
	Link    string
}
