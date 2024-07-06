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

var BOOK_LIMIT int = 100
var NUM_SCRAPERS int = 10
var book_count int = 0

var mutex = &sync.Mutex{}

func scrapeLinks(summary_links []string, wg *sync.WaitGroup, book_links chan<- []string) {
	defer wg.Done()

	var collected_book_links []string

	c := colly.NewCollector()

	c.OnHTML("h2", func(e *colly.HTMLElement) {
		linkText := strings.TrimSpace(e.Text)
		if strings.Contains(linkText, "(English)") {
			e.DOM.Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
				if book_count >= BOOK_LIMIT {
					return
				}
				link, _ := sel.Attr("href")
				book_count = book_count + 1
				fmt.Println(book_count)
				collected_book_links = append(collected_book_links, link)
			})
		}
	})

	for _, summary_link := range summary_links {
		c.Visit(summary_link)
	}

	book_links <- collected_book_links
}

func get_indexes_links() []string {
	c := colly.NewCollector()

	var scraped_links []string

	c.OnHTML("div.pgdbnavbar > p:nth-of-type(2) > a", func(e *colly.HTMLElement) {
		link := "https://www.gutenberg.org" + e.Attr("href")
		scraped_links = append(scraped_links, link)
	})

	c.Visit("https://www.gutenberg.org/browse/languages/en")

	return scraped_links
}

func divide_index_links(indexes_link_list []string) [][]string {
	portionSize := (len(indexes_link_list) + NUM_SCRAPERS - 1) / NUM_SCRAPERS
	var portions [][]string

	for i := 0; i < len(indexes_link_list); i += portionSize {
		end := i + portionSize
		if end > len(indexes_link_list) {
			end = len(indexes_link_list)
		}
		portions = append(portions, indexes_link_list[i:end])
	}

	return portions
}

func extract_book_ids(book_links []string) map[string]string {
	bookIDToLink := make(map[string]string)

	for _, book_link := range book_links {
		lastSlashIndex := strings.LastIndex(book_link, "/")
		if lastSlashIndex != -1 {
			book_id := book_link[lastSlashIndex+1:]
			formatted_link := fmt.Sprintf("https://www.gutenberg.org/cache/epub/%s/pg%s.txt", book_id, book_id)
			bookIDToLink[book_id] = formatted_link
		}
	}

	return bookIDToLink
}

func fetchAndSaveBooks(links map[string]string) {
	var wg sync.WaitGroup

	for bookID, link := range links {
		wg.Add(1)
		go func(bookID, link string) {
			defer wg.Done()

			resp, err := http.Get(link)
			if err != nil {
				fmt.Printf("Error fetching book %s: %v\n", bookID, err)
				time.Sleep(1 * time.Second)
				return
			}
			defer resp.Body.Close()

			// Read response body
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				fmt.Printf("Error reading response body for book %s: %v\n", bookID, err)
				time.Sleep(1 * time.Second)
				return
			}

			// Save to file
			mutex.Lock()
			defer mutex.Unlock()

			dir := "./books"
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				err := os.Mkdir(dir, 0755)
				if err != nil {
					fmt.Printf("Error creating directory %s: %v\n", dir, err)
					return
				}
			}

			filePath := filepath.Join(dir, fmt.Sprintf("%s.txt", bookID))
			err = ioutil.WriteFile(filePath, body, 0644)
			if err != nil {
				fmt.Printf("Error writing file %s: %v\n", filePath, err)
				return
			}

			fmt.Printf("Book %s saved successfully.\n", bookID)

			// Introduce a delay after each request
			time.Sleep(1 * time.Second)
		}(bookID, link)
	}

	wg.Wait()
}

func main() {
	var indexes_link_list []string = get_indexes_links()

	var portions [][]string = divide_index_links(indexes_link_list)
	var wg sync.WaitGroup

	book_links := make(chan []string, NUM_SCRAPERS)

	// Start scrapers
	for _, portion := range portions {
		wg.Add(1)
		go scrapeLinks(portion, &wg, book_links)
	}

	// Wait for all goroutines to finish
	go func() {
		wg.Wait()
		close(book_links)
	}()

	// Collect all results
	var allLinks []string
	for link := range book_links {
		allLinks = append(allLinks, link...)
	}

	// Extract book IDs and formatted links
	bookIDToLink := extract_book_ids(allLinks)

	// Fetch and save books
	fetchAndSaveBooks(bookIDToLink)
}
