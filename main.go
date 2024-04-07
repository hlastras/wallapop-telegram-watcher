package main

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
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
	Price int
	Title string
}

type ItemRecord struct {
	urlHash         string `json:"urlHash"`
	itemID          string `json:"itemID"`
	price           int    `json:"price"`
	updateTimestamp string `json:"updateTimestamp"`
}

func main() {
	c := cron.New()
	_, _ = c.AddFunc("*/1 * * * *", runAnalysis)
	c.Start()

	select {}
}

func runAnalysis() {
	fmt.Println("Executing analysis...")
	defer fmt.Println("Analysis complete.")

	// Load URLs from config.json
	urls, err := loadURLsFromConfig()
	if err != nil {
		log.Fatalf("Failed to load URLs from config: %v", err)
	}

	// Read the existing CSV file into memory
	records, err := readCSVFile("./config/analysis_results.csv")
	if err != nil {
		log.Fatalf("Failed to read CSV file: %v", err)
	}

	// Convert records to a map for easy lookup
	recordsMap := make(map[string]ItemRecord)
	for _, record := range records {
		recordsMap[record.urlHash+record.itemID] = record
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

		urlHash := hashURL(urlToSearch)
		fmt.Println("URL Hash:", urlHash, "Items:", len(items))
		for _, item := range items {
			itemID := item.Link // Assuming the link is unique for each item
			key := urlHash + itemID
			currentTime := time.Now().Format(time.RFC3339)

			// Check if the item exists in the recordsMap
			if record, exists := recordsMap[key]; exists {
				// If the price has changed, update the record
				if record.price != item.Price {
					// TODO: message to alert about a change
					fmt.Printf("PRICE CHANGE:\n\tLink: %s\n\tPrice: %d\n\tOld Price: %d\n\tTitle: %s\n\n", item.Link, item.Price, record.price, item.Title)
					record.price = item.Price
					record.updateTimestamp = currentTime
					recordsMap[key] = record
				}
			} else {
				// If the item is new, add it to the recordsMap
				// TODO: message to alert about a new item
				fmt.Printf("NEW ITEM:\n\tLink: %s\n\tPrice: %d\n\tTitle: %s\n\n", item.Link, item.Price, item.Title)
				recordsMap[key] = ItemRecord{
					urlHash:         urlHash,
					itemID:          itemID,
					price:           item.Price,
					updateTimestamp: currentTime,
				}
			}
		}
	}

	// Convert recordsMap back to a slice of ItemRecord
	updatedRecords := make([]ItemRecord, 0, len(recordsMap))
	for _, record := range recordsMap {
		updatedRecords = append(updatedRecords, record)
	}

	// Write the updated records back to the CSV file
	if err := writeCSVFile("./config/analysis_results.csv", updatedRecords); err != nil {
		log.Fatalf("Failed to write CSV file: %v", err)
	}
}

func loadURLsFromConfig() ([]string, error) {
	file, err := os.Open("./config/config.json")
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
		priceText := s.Find(".ItemCard__price").Text()
		priceText = strings.TrimSpace(priceText)
		// Convert price from string to integer, assuming price is in the format "360,00\u00a0€"
		price, err := convertPriceToInt(priceText)
		if err != nil {
			log.Printf("Error converting price to integer for %s: %v", link, err)
			return
		}

		items = append(items, Item{
			Link:  link,
			Price: price,
			Title: title,
		})
	})

	return items, nil
}

func convertPriceToInt(priceText string) (int, error) {
	// Remove the currency symbol and any spaces or non-breaking spaces
	cleanPrice := strings.TrimSpace(priceText)                // Remove leading/trailing spaces
	cleanPrice = strings.ReplaceAll(cleanPrice, "\u00a0", "") // Remove non-breaking spaces
	cleanPrice = strings.ReplaceAll(cleanPrice, "€", "")      // Remove the currency symbol

	// Replace comma with nothing (discard cents)
	cleanPrice = strings.Split(cleanPrice, ",")[0]

	// Convert to integer
	price, err := strconv.Atoi(cleanPrice)
	if err != nil {
		return 0, err
	}
	return price, nil
}

func writeCSVFile(filePath string, records []ItemRecord) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error creating CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	for _, record := range records {
		row := []string{
			record.urlHash,
			record.itemID,
			strconv.Itoa(record.price),
			record.updateTimestamp,
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("error writing record to CSV file: %w", err)
		}
	}

	return nil
}

// readCSVFile reads the CSV file and returns a slice of ItemRecord.
// If the file does not exist, it returns an empty slice and no error.
func readCSVFile(filePath string) ([]ItemRecord, error) {
	var records []ItemRecord

	// Check if the file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return records, nil // File does not exist, return empty slice
	}

	// Open the CSV file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("error opening CSV file: %w", err)
	}
	defer file.Close()

	// Create a new CSV reader
	reader := csv.NewReader(file)
	rawCSVdata, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("error reading CSV file: %w", err)
	}

	// Parse the CSV data into ItemRecord structs
	for _, row := range rawCSVdata {
		price, _ := strconv.Atoi(row[2]) // Convert price to integer
		record := ItemRecord{
			urlHash:         row[0],
			itemID:          row[1],
			price:           price,
			updateTimestamp: row[3],
		}
		records = append(records, record)
	}

	return records, nil
}

// hashURL takes a URL string and returns the first 6 characters of its SHA256 hash.
func hashURL(url string) string {
	hasher := sha256.New()
	hasher.Write([]byte(url))
	fullHash := hex.EncodeToString(hasher.Sum(nil))
	return fullHash[:6]
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
