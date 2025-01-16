package main

import (
	"fmt"

	"github.com/gocolly/colly"
)

// initialize a data structure to keep the scraped data
type Job struct {
	Url, Name, Location string
}

func main() {
	c := colly.NewCollector()

	var jobs []Job

	// OnHTML callback
	c.OnHTML(".job-post", func(e *colly.HTMLElement) {
		// initialize a new Job instance
		job := Job{}

		fmt.Println(e.DOM)
		// scrape the target data
		job.Url = e.ChildAttr("a", "href")

		//job.Name = e.
		job.Location = e.ChildText(".price")

		// add the product instance with scraped data to the list of products
		jobs = append(jobs, job)
	})
	c.Visit("https://job-boards.greenhouse.io/defenseunicorns")

	for _, job := range jobs {
		fmt.Println(job)
	}
}
