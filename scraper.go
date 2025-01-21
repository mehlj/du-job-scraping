package main

import (
	"fmt"

	"github.com/gocolly/colly"
)

type Job struct {
	Title, Location, Url string
}

func main() {
	c := colly.NewCollector()

	var jobs []Job

	c.OnHTML(".job-post", func(e *colly.HTMLElement) {
		job := Job{}

		job.Title = e.ChildText("p.body.body--medium")
		job.Location = e.ChildText("p.body.body__secondary.body--metadata")
		job.Url = e.ChildAttr("a", "href")

		jobs = append(jobs, job)
	})
	c.Visit("https://job-boards.greenhouse.io/defenseunicorns")

	for _, job := range jobs {
		fmt.Println(job)
	}

	// TODO store jobs in S3 for diff comparison
}
