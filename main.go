package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ragoob/quiet_hn/hn"
)

var (
	mu    sync.Mutex
	cache map[int]item
)

func main() {
	cache = make(map[int]item)
	// parse flags
	var port, numStories int
	flag.IntVar(&port, "port", 3000, "the port to start the web server on")
	flag.IntVar(&numStories, "num_stories", 30, "the number of top stories to display")
	flag.Parse()

	tpl := template.Must(template.ParseFiles("./index.gohtml"))

	http.HandleFunc("/", handler(numStories, tpl))

	// Start the server
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func handler(numStories int, tpl *template.Template) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		var client hn.Client
		ids, err := client.TopItems()
		if err != nil {
			http.Error(w, "Failed to load top stories", http.StatusInternalServerError)
			return
		}
		var stories []item
		var wg sync.WaitGroup
		total := int32(float64(numStories) * 1.25)
		ids = ids[0:total]
		for _, id := range ids {
			mu.Lock()
			v, ok := cache[id]
			if ok {
				fmt.Printf("Item with Id [%d] cached \n", id)
				stories = append(stories, v)
				mu.Unlock()
				continue
			}
			mu.Unlock()

			fmt.Printf("Looking for Item [%d] from service \n", id)
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				hnItem, err := client.GetItem(id)
				if err != nil {
					return
				}
				item := parseHNItem(hnItem)
				if isStoryLink(item) {

					mu.Lock()

					if len(stories) < numStories {
						stories = append(stories, item)
						cache[id] = item
					}

					mu.Unlock()

				}
			}(id)
		}
		wg.Wait()
		sort.Slice(stories, func(i, j int) bool {
			return stories[i].ID < stories[j].ID
		})
		data := templateData{
			Stories: stories,
			Time:    time.Now().Sub(start),
		}
		err = tpl.Execute(w, data)
		if err != nil {
			http.Error(w, "Failed to process the template", http.StatusInternalServerError)
			return
		}
	})
}

func isStoryLink(item item) bool {
	return item.Type == "story" && item.URL != ""
}

func parseHNItem(hnItem hn.Item) item {
	ret := item{Item: hnItem}
	url, err := url.Parse(ret.URL)
	if err == nil {
		ret.Host = strings.TrimPrefix(url.Hostname(), "www.")
	}
	return ret
}

// item is the same as the hn.Item, but adds the Host field
type item struct {
	hn.Item
	Host string
}

type templateData struct {
	Stories []item
	Time    time.Duration
}
