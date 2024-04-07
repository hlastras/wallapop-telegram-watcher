package main

import (
	"context"
	"fmt"
	"log"
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
	urlToSearch := "https://es.wallapop.com/app/search?latitude=40.36660736519385&longitude=-3.763628939911825&keywords=Amazon%20kindle%20scribe&min_sale_price=200&max_sale_price=400&order_by=newest&country_code=ES&filters_source=stored_filters"

	session := buildChromeSession()

	htmlContent, err := getUrlHTMLContent(session, urlToSearch)
	if err != nil {
		log.Fatalf("Failed to get HTML content: %v", err)
	}

	regex := regexp.MustCompile(`<tsl-item-card-images-carousel[^>]*>([\s\S]*?)</tsl-item-card-images-carousel>`)

	sanitizeHTML := regex.ReplaceAllString(htmlContent, "")
	items, err := parseHTML(sanitizeHTML)
	if err != nil {
		log.Fatalf("Error parsing HTML: %v", err)
	}

	for _, item := range items {
		fmt.Printf("Link: %s\nPrice: %s\nTitle: %s\n\n", item.Link, item.Price, item.Title)
	}
	fmt.Println("Total items found:", len(items))
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
