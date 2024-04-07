package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"github.com/robfig/cron/v3"
)

type ChromeSession struct {
	ctx            context.Context
	allocCtx       context.Context
	cancelAllocCtx context.CancelFunc
	cancelCtx      context.CancelFunc
}

type Item struct {
	Link  string
	Price string
	Title string
}

func main() {
	c := cron.New()
	_, _ = c.AddFunc("*/1 * * * *", runAnalysis)
	c.Start()

	select {}
}

func runAnalysis() {
	fmt.Println("Executing analysis...")

	// Load URLs from config.json
	urls, err := loadURLsFromConfig()
	if err != nil {
		log.Fatalf("Failed to load URLs from config: %v", err)
	}

	session := buildChromeSession()

	for _, urlToSearch := range urls {
		htmlContent, err := getUrlHTMLContent(session, urlToSearch)
		if err != nil {
			log.Printf("Failed to get HTML content for %s: %v", urlToSearch, err)
			continue
		}

		regex := regexp.MustCompile(`<tsl-item-card-images-carousel[^>]*>([\s\S]*?)</tsl-item-card-images-carousel>`)

		sanitizeHTML := regex.ReplaceAllString(htmlContent, "")
		items, err := parseHTML(sanitizeHTML)
		if err != nil {
			log.Printf("Error parsing HTML for %s: %v", urlToSearch, err)
			continue
		}

		for _, item := range items {
			fmt.Printf("Link: %s\nPrice: %s\nTitle: %s\n\n", item.Link, item.Price, item.Title)
		}
		fmt.Println("Total items found for", urlToSearch, ":", len(items))
	}
}

func loadURLsFromConfig() ([]string, error) {
	file, err := os.Open("config.json")
	if err != nil {
		return nil, fmt.Errorf("error opening config file: %w", err)
	}
	defer file.Close()

	var config struct {
		URLs []string `json:"urls"`
	}
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, fmt.Errorf("error decoding config file: %w", err)
	}

	return config.URLs, nil
}

func parseHTML(html string) ([]Item, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var items []Item
	doc.Find("a.ItemCardList__item").Each(func(i int, s *goquery.Selection) {
		title, _ := s.Attr("title")
		link, _ := s.Attr("href")

		price := s.Find(".ItemCard__price").Text()
		price = strings.TrimSpace(price)

		items = append(items, Item{
			Link:  link,
			Price: price,
			Title: title,
		})
	})

	return items, nil
}

func buildChromeSession() *ChromeSession {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
	)
	allocCtx, cancelAllocCtx := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelCtx := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))

	return &ChromeSession{ctx: ctx, allocCtx: allocCtx, cancelAllocCtx: cancelAllocCtx, cancelCtx: cancelCtx}
}

func getUrlHTMLContent(session *ChromeSession, url string) (string, error) {
	var content string
	err := chromedp.Run(session.ctx,
		chromedp.Navigate(url),
		chromedp.Sleep(3*time.Second),
		chromedp.OuterHTML("html", &content),
	)
	if err != nil {
		log.Fatal(err)
	}
	return content, err
}
