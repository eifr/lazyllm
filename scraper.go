package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ScrapeModels fetches the base list of models from ollama.com/library
func ScrapeModels() ([]string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://ollama.com/library")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", resp.StatusCode, resp.Status)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var models []string
	// The bash script looks for 'class="group"' and extracts the href
	// We can more specifically target the a tags that link to /library/
	doc.Find("a[href^='/library/']").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			parts := strings.Split(href, "/")
			if len(parts) >= 3 && parts[1] == "library" {
				model := parts[2]
				// Avoid duplicates and non-model links
				if !contains(models, model) && model != "" {
					models = append(models, model)
				}
			}
		}
	})

	sort.Strings(models)
	return models, nil
}

// ScrapeTags fetches the specific pullable tags for a single model
func ScrapeTags(model string) ([]string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://ollama.com/library/%s/tags", model)
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", resp.StatusCode, resp.Status)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var tags []string
	// Look for the elements that contain "ollama pull <tag>"
	doc.Find("*").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if strings.HasPrefix(text, "ollama pull ") {
			tag := strings.TrimPrefix(text, "ollama pull ")
			// Clean up any potential HTML artifacts
			tag = strings.Split(tag, "<")[0]
			tag = strings.TrimSpace(tag)
			if !contains(tags, tag) && tag != "" {
				tags = append(tags, tag)
			}
		}
	})

	// Fallback: If no explicit pull commands found, usually the model name itself is the default tag
	if len(tags) == 0 {
		tags = append(tags, model)
	}

	return tags, nil
}

// ScrapeAll fetch all models and their tags concurrently
func ScrapeAll(progressChan chan string) ([]string, error) {
	models, err := ScrapeModels()
	if err != nil {
		return nil, err
	}

	var allTags []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Use a semaphore to limit concurrent HTTP requests to avoid rate limits
	sem := make(chan struct{}, 10)

	for _, m := range models {
		wg.Add(1)
		go func(modelName string) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire token
			defer func() { <-sem }() // Release token

			if progressChan != nil {
				progressChan <- fmt.Sprintf("Gathering tags for %s...", modelName)
			}

			tags, err := ScrapeTags(modelName)
			if err == nil {
				mu.Lock()
				allTags = append(allTags, tags...)
				mu.Unlock()
			}
		}(m)
	}

	wg.Wait()
	sort.Strings(allTags)
	return allTags, nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
