package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
)

func fetchChannelDataWithColly(username string) (*ChannelData, error) {
	// Add random delay before request
	delay := time.Duration(2+rand.Intn(3)) * time.Second
	fmt.Printf("  - Waiting %v before request...\n", delay)
	time.Sleep(delay)

	c := colly.NewCollector(
		colly.AllowURLRevisit(),
		colly.Async(false),
	)

	// Set random user agent
	userAgents := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:89.0) Gecko/20100101 Firefox/89.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.1.1 Safari/605.1.15",
	}
	
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("User-Agent", userAgents[rand.Intn(len(userAgents))])
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
		r.Headers.Set("Accept-Language", "en-US,en;q=0.9,fa;q=0.8")
		r.Headers.Set("DNT", "1")
		r.Headers.Set("Connection", "keep-alive")
	})

	channelData := &ChannelData{
		Info: ChannelInfo{
			Username: username,
		},
		Posts: []Post{},
	}

	// Extract channel info
	c.OnHTML(`meta[property="og:title"]`, func(e *colly.HTMLElement) {
		channelData.Info.Title = e.Attr("content")
	})

	c.OnHTML(`meta[property="og:image"]`, func(e *colly.HTMLElement) {
		channelData.Info.Photo = e.Attr("content")
	})

	// Extract posts
	c.OnHTML(`.tgme_widget_message_wrap`, func(e *colly.HTMLElement) {
		post := Post{}
		
		// Extract post ID
		if postIDStr := e.Attr("data-post"); postIDStr != "" {
			parts := strings.Split(postIDStr, "/")
			if len(parts) > 1 {
				fmt.Sscanf(parts[1], "%d", &post.ID)
			}
		}

		// Extract message text (keep HTML for markdown)
		e.ForEach(".tgme_widget_message_text", func(i int, textElem *colly.HTMLElement) {
			// Get HTML content to preserve markdown formatting
			messageHTML, _ := textElem.DOM.Html()
			post.Message = strings.TrimSpace(messageHTML)
		})

		// Extract date with correct format
		e.ForEach("time", func(i int, timeElem *colly.HTMLElement) {
			if datetime := timeElem.Attr("datetime"); datetime != "" {
				// Handle format: "2026-04-07T01:21:57+00:00"
				if parsedTime, err := time.Parse("2006-01-02T15:04:05-07:00", datetime); err == nil {
					post.Date = parsedTime
				} else if parsedTime, err := time.Parse("2006-01-02T15:04:05+00:00", datetime); err == nil {
					post.Date = parsedTime
				} else if parsedTime, err := time.Parse(time.RFC3339, datetime); err == nil {
					post.Date = parsedTime
				} else {
					// Debug: print the datetime format we couldn't parse
					fmt.Printf("DEBUG: Could not parse date: %s\n", datetime)
				}
			}
		})

		// Extract views
		e.ForEach(".tgme_widget_message_views", func(i int, viewsElem *colly.HTMLElement) {
			viewsText := strings.TrimSpace(viewsElem.Text)
			if viewsText != "" {
				// Parse views (handle K, M suffixes)
				if strings.HasSuffix(viewsText, "K") {
					var views float64
					fmt.Sscanf(strings.TrimSuffix(viewsText, "K"), "%f", &views)
					post.Views = int(views * 1000)
				} else {
					fmt.Sscanf(viewsText, "%d", &post.Views)
				}
			}
		})

		// Extract sender info
		e.ForEach(".tgme_widget_message_owner_name", func(i int, senderElem *colly.HTMLElement) {
			post.SenderName = strings.TrimSpace(senderElem.Text)
		})

		// Extract media info
		e.ForEach(".tgme_widget_message_photo_wrap", func(i int, photoWrap *colly.HTMLElement) {
			// Extract photo from background-image style
			if style := photoWrap.Attr("style"); style != "" {
				// Look for background-image:url('...')
				if strings.Contains(style, "background-image:url") {
					start := strings.Index(style, "url('")
					if start != -1 {
						start += 5 // skip "url('"
						end := strings.Index(style[start:], "')")
						if end != -1 {
							imgURL := style[start : start+end]
							media := Media{
								Type: "photo",
								URL:  imgURL,
							}
							post.Media = append(post.Media, media)
						}
					}
				}
			}
		})

		// Extract uploaded videos
		e.ForEach(".tgme_widget_message_video_player", func(i int, videoPlayer *colly.HTMLElement) {
			if videoURL := videoPlayer.Attr("href"); videoURL != "" {
				media := Media{
					Type: "video",
					URL:  videoURL,
				}
				
				if thumbElem := videoPlayer.DOM.Find(".tgme_widget_message_video_thumb"); thumbElem.Length() > 0 {
					if style, exists := thumbElem.Attr("style"); exists && style != "" {
						if strings.Contains(style, "background-image:url") {
							start := strings.Index(style, "url('")
							if start != -1 {
								start += 5
								end := strings.Index(style[start:], "')")
								if end != -1 {
									thumbURL := style[start : start+end]
									media.URL = thumbURL
								}
							}
						}
					}
				}
				
				if videoWrap := videoPlayer.DOM.Find(".tgme_widget_message_video_wrap"); videoWrap.Length() > 0 {
					if style, exists := videoWrap.Attr("style"); exists && style != "" {
						if strings.Contains(style, "width:") {
							start := strings.Index(style, "width:") + 6
							end := strings.Index(style[start:], "px")
							if end != -1 {
								fmt.Sscanf(style[start:start+end], "%d", &media.Width)
							}
						}
					}
				}
				
				post.Media = append(post.Media, media)
			}
		})

		// Extract document files
		e.ForEach(".tgme_widget_message_document", func(i int, docElem *colly.HTMLElement) {
			if docURL := docElem.ChildAttr("a", "href"); docURL != "" {
				media := Media{
					Type:     "document",
					URL:      docURL,
					FileName: strings.TrimSpace(docElem.ChildText(".tgme_widget_message_document_title")),
				}
				post.Media = append(post.Media, media)
			}
		})

		if post.ID > 0 || post.Message != "" {
			if post.Caption == "" {
				post.Caption = ""
			}
			channelData.Posts = append(channelData.Posts, post)
		}
	})

	// Load more posts (up to 5 pages = ~100 posts)
	pageCount := 1
	c.OnHTML(`.tme_messages_more`, func(e *colly.HTMLElement) {
		if pageCount >= 5 {
			return
		}
		if nextLink := e.Attr("href"); nextLink != "" {
			pageCount++
			fullNextLink := "https://t.me" + nextLink
			time.Sleep(3 * time.Second)
			c.Visit(fullNextLink)
		}
	})

	// Visit the page
	url := fmt.Sprintf("https://t.me/s/%s", username)
	err := c.Visit(url)
	if err != nil {
		return nil, fmt.Errorf("failed to visit page: %v", err)
	}

	c.Wait()

	channelData.LastUpdated = time.Now().Unix()
	return channelData, nil
}
