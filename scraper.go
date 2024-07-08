package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
)

const (
	BOOKS_DIR     = "./books"
	BOOK_LIMIT    = 10000
	NUM_WORKERS   = 3
	MAX_RETRIES   = 3
	RETRY_DELAY   = 2 * time.Second
	REQUEST_DELAY = 100 * time.Millisecond
)

var (
	book_count int
	mutex      sync.Mutex
	sem        = make(chan struct{}, NUM_WORKERS)
	quit       = make(chan bool)
	once       sync.Once
)

func create_books_dir() {
	if _, err := os.Stat(BOOKS_DIR); os.IsNotExist(err) {
		os.Mkdir(BOOKS_DIR, 0755)
	}
}

func scrape_links(summary_links []string, wg *sync.WaitGroup, book_links chan<- []string) {
	defer wg.Done()

	var collected_book_links []string
	c := colly.NewCollector()

	c.OnHTML("h2", func(e *colly.HTMLElement) {
		link_text := strings.TrimSpace(e.Text)
		if strings.Contains(link_text, "(English)") {
			e.DOM.Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
				link, _ := sel.Attr("href")
				collected_book_links = append(collected_book_links, link)
			})
		}
	})

	for _, summary_link := range summary_links {
		c.Visit(summary_link)
	}

	book_links <- collected_book_links
}

func get_indexes_link_list() []string {
	c := colly.NewCollector()
	var scraped_links []string

	c.OnHTML("div.pgdbnavbar > p:nth-of-type(2) > a", func(e *colly.HTMLElement) {
		link := "https://www.gutenberg.org" + e.Attr("href")
		scraped_links = append(scraped_links, link)
	})

	c.Visit("https://www.gutenberg.org/browse/languages/en")

	return scraped_links
}

func divide_indexes_link_list(indexes_links_list []string) [][]string {
	portion_size := (len(indexes_links_list) + NUM_WORKERS - 1) / NUM_WORKERS
	var portions [][]string

	for i := 0; i < len(indexes_links_list); i += portion_size {
		end := i + portion_size
		if end > len(indexes_links_list) {
			end = len(indexes_links_list)
		}
		portions = append(portions, indexes_links_list[i:end])
	}

	return portions
}

func extractbook_ids(book_links []string) map[string]string {
	book_info := make(map[string]string)

	for _, book_link := range book_links {
		last_slash_index := strings.LastIndex(book_link, "/")
		if last_slash_index != -1 {
			book_id := book_link[last_slash_index+1:]
			formatted_link := fmt.Sprintf("https://www.gutenberg.org/cache/epub/%s/pg%s.txt", book_id, book_id)
			book_info[book_id] = formatted_link
		}
	}

	return book_info
}

func fetch_books(links map[string]string) {
	var wg sync.WaitGroup

	for book_id, link := range links {
		wg.Add(1)
		go func(book_id, link string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if fetch_and_save_books(book_id, link) {
				mutex.Lock()
				book_count++
				if book_count >= BOOK_LIMIT {
					once.Do(func() { close(quit) })
				}
				mutex.Unlock()
			}
		}(book_id, link)
	}

	wg.Wait()
}

func fetch_and_save_books(book_id, link string) bool {
	for retry := 0; retry < MAX_RETRIES; retry++ {
		select {
		case <-quit:
			return false
		default:
			if success := handle_book_request(book_id, link); success {
				return true
			}
			time.Sleep(RETRY_DELAY)
		}
	}

	fmt.Printf("Failed to fetch and save book %s after %d retries.\n", book_id, MAX_RETRIES)
	return false
}

func handle_book_request(book_id, link string) bool {
	time.Sleep(REQUEST_DELAY)
	resp, err := http.Get(link)
	if err != nil {
		fmt.Printf("Error fetching book %s: %v\n", book_id, err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Printf("Book %s not found (404).\n", book_id)
		return false
	} else if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error fetching book %s: HTTP status %s\n", book_id, resp.Status)
		return false
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response body for book %s: %v\n", book_id, err)
		return false
	}
	if BOOK_LIMIT > book_count {
		if err := save_book_file(book_id, body); err != nil {
			fmt.Printf("Error saving book %s: %v\n", book_id, err)
			return false
		}
		fmt.Printf("Book %s saved successfully.\n", book_id)
	}
	return true
}

func save_book_file(file_name string, content []byte) error {
	file_path := filepath.Join(BOOKS_DIR, fmt.Sprintf("%s.txt", file_name))
	return ioutil.WriteFile(file_path, content, 0644)
}

func main() {

	create_books_dir()

	indexes_links_list := get_indexes_link_list()
	portions := divide_indexes_link_list(indexes_links_list)
	var wg sync.WaitGroup

	book_links := make(chan []string, NUM_WORKERS)

	for _, portion := range portions {
		wg.Add(1)
		go scrape_links(portion, &wg, book_links)
	}

	go func() {
		wg.Wait()
		close(book_links)
	}()

	var all_book_links []string
	for link := range book_links {
		all_book_links = append(all_book_links, link...)
	}

	book_info := extractbook_ids(all_book_links)

	fetch_books(book_info)
}
