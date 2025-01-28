package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/gocolly/colly"
	"github.com/nsf/jsondiff"
	"gopkg.in/gomail.v2"
)

// a Job represents a single job posting from Defense Unicorns
type Job struct {
	Title    string `json:"title"`
	Location string `json:"location"`
	Url      string `json:"url"`
}

// Defense Unicorns job posting site constants
const (
	JOB_URL    = "https://job-boards.greenhouse.io/defenseunicorns"
	JOB_DOMAIN = "job-boards.greenhouse.io"
	FILE_NAME  = "jobs.json"
)

// Scrapes the Defense Unicorns job site for all open postings
// Returns a slice[] of Jobs
func getJobs() []Job {
	var jobs []Job
	c := colly.NewCollector(
		colly.AllowedDomains(JOB_DOMAIN),
	)

	c.OnHTML(".job-post", func(e *colly.HTMLElement) {
		job := Job{}

		job.Title = e.ChildText("p.body.body--medium")
		job.Location = e.ChildText("p.body.body__secondary.body--metadata")
		job.Url = e.ChildAttr("a", "href")

		jobs = append(jobs, job)
	})
	c.Visit(JOB_URL)

	return jobs
}

// getFromS3 reads the jobList from S3, returns nil if nonexistant
// Accepts context and a prebuilt S3 client
func getFromS3(ctx context.Context, client *s3.Client, bucketName string) string {
	result, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(FILE_NAME),
	})
	// catch S3 error
	if err != nil {
		var noKey *types.NoSuchKey
		if errors.As(err, &noKey) {
			log.Printf("Can't get object %s from bucket %s. No such key exists.\n", FILE_NAME, bucketName)
		} else {
			log.Printf("Couldn't get object %v:%v. Here's why: %v\n", bucketName, FILE_NAME, err)
		}
		return ""
	}

	defer result.Body.Close()

	// return file contents
	body, err := io.ReadAll(result.Body)
	if err != nil {
		log.Printf("Couldn't read object body from %v. Here's why: %v\n", FILE_NAME, err)
	}
	return string(body)
}

// uploadToS3 uploads the jobList to the S3 bucket
// Accepts context and a prebuilt S3 client
func uploadToS3(ctx context.Context, client *s3.Client, bucketName string) error {
	file, err := os.Open(FILE_NAME)
	if err != nil {
		log.Printf("Couldn't open file %v to upload. Here's why: %v\n", FILE_NAME, err)
	} else {
		defer file.Close()
		_, err = client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(FILE_NAME),
			Body:   file,
		})
		if err != nil {
			log.Printf("Couldn't upload file %v to %v:%v. Here's why: %v\n",
				FILE_NAME, bucketName, FILE_NAME, err)
		} else {
			err = s3.NewObjectExistsWaiter(client).Wait(
				ctx, &s3.HeadObjectInput{Bucket: aws.String(bucketName), Key: aws.String(FILE_NAME)}, time.Minute)
			if err != nil {
				log.Printf("Failed attempt to wait for object %s to exist.\n", FILE_NAME)
			}
		}
	}
	return err
}

// converts jobList struct to a JSON file for uploading to S3
func convertJobListToFile(jobs []Job) {
	data, err := json.MarshalIndent(jobs, "", " ")

	if err != nil {
		log.Printf("Failed to marshall jobList to JSON")
		panic(err)
	}

	err = os.WriteFile(FILE_NAME, data, 0644)
	if err != nil {
		log.Printf("Failed to save jobList[] to %s\n", FILE_NAME)
		panic(err)
	}
}

// reads jobList from file as JSON
func readJobListFromFile() string {
	data, err := os.ReadFile(FILE_NAME)
	if err != nil {
		log.Printf("Failed to read jobList[] from %s\n", FILE_NAME)
		panic(err)
	}

	// Convert the byte slice to a string
	return string(data)
}

// performs diff on new and old job postings
func jobListDiff(lastJobPostings string) string {
	// read currentJobs from file
	currentJobs := readJobListFromFile()
	diff := ""

	if currentJobs != lastJobPostings {
		options := jsondiff.DefaultHTMLOptions()
		_, json := jsondiff.Compare([]byte(lastJobPostings), []byte(currentJobs), &options)
		diff = json
	} else {
		fmt.Println("No job posting updates.")
	}
	return diff
}

// returns secret from AWS Secrets Manager
func getSecret(secretName string) string {
	region := "us-east-1"

	config, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		log.Fatal(err)
	}

	// create secrets manager client
	svc := secretsmanager.NewFromConfig(config)

	input := &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(secretName),
		VersionStage: aws.String("AWSCURRENT"), // VersionStage defaults to AWSCURRENT if unspecified
	}

	result, err := svc.GetSecretValue(context.TODO(), input)
	if err != nil {
		// For a list of exceptions thrown, see
		// https://<<{{DocsDomain}}>>/secretsmanager/latest/apireference/API_GetSecretValue.html
		log.Fatal(err.Error())
	}

	// Decrypts secret using the associated KMS key.
	var secretString string = *result.SecretString
	return secretString
}

func getS3BucketName(ctx context.Context, client *ssm.Client) string {
	parameterName := "/du-job-scraping/s3BucketName"

	output, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(parameterName),
		WithDecryption: aws.Bool(false), // Set to true if it's a SecureString
	})
	if err != nil {
		log.Fatalf("Failed to get parameter: %v", err)
	}

	bucketName := *output.Parameter.Value
	return bucketName
}

func sendEmail(body string) {
	address := "justenmehl12@gmail.com"
	password := getSecret("GMAIL_APP_PASSWORD")
	fmt.Println(password)

	message := gomail.NewMessage()

	message.SetHeader("From", address)
	message.SetHeader("To", address)
	message.SetHeader("Subject", "Defense Unicorns Job Change")

	// Build the HTML content
	htmlContent := `
		<html>
		<body>
			<h1 style="color:blue;">Job Changes</h1>
			<pre style="font-family:monospace;">` + body + `</pre>
		</body>
		</html>`

	message.SetBody("text/html", htmlContent)

	dialer := gomail.NewDialer("smtp.gmail.com", 587, address, password)

	// Send the email
	if err := dialer.DialAndSend(message); err != nil {
		fmt.Println("Failed to send email:", err)
		panic(err)
	} else {
		log.Println("Successfully sent to " + address)
	}
}

func main() {
	// parse DU job site for current postings, store in local JSON
	currentJobs := getJobs()
	convertJobListToFile(currentJobs)

	// load AWS config
	ctx := context.Background()
	sdkConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		fmt.Println("Couldn't load default configuration. Missing AWS environment variables.")
		fmt.Println(err)
		return
	}

	// find S3 bucket name from pulumi output
	ssmClient := ssm.NewFromConfig(sdkConfig)
	bucketName := getS3BucketName(ctx, ssmClient)

	// check for jobPosting changes
	s3Client := s3.NewFromConfig(sdkConfig)

	lastJobPostings := getFromS3(ctx, s3Client, bucketName)
	if lastJobPostings == "" {
		fmt.Println("Job listing in S3 is not present. Uploading..")

		err = uploadToS3(ctx, s3Client, bucketName)
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Printf("Upload to %s successful.\n", bucketName)
		}
	} else {
		fmt.Println("Doing diff..")
		diff := jobListDiff(lastJobPostings)

		if diff != "" {
			fmt.Println(diff)
			fmt.Println("Changes detected. Sending email and updating S3..")

			// update S3 with posting changes
			err = uploadToS3(ctx, s3Client, bucketName)
			if err != nil {
				fmt.Println(err)
			} else {
				fmt.Printf("Upload to %s successful.\n", bucketName)
			}

			// notify myself with changes
			sendEmail(diff)
		}
	}
}
