package main

import (
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ssm"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/s3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// create S3 bucket for storing scraped data
		bucket, err := s3.NewBucketV2(ctx, "du-scraping-bucket", nil)
		if err != nil {
			return err
		}

		// store bucket name in parameter store, so app code can access
		_, err = ssm.NewParameter(ctx, "bucketNameParam", &ssm.ParameterArgs{
			Name:  pulumi.String("/du-job-scraping/s3BucketName"),
			Type:  pulumi.String("String"),
			Value: bucket.ID(),
		})
		if err != nil {
			return err
		}
		ctx.Log.Info("Successfully stored S3 bucket name in Parameter Store.", nil)

		// Export the name of the bucket
		ctx.Export("bucketName", bucket.ID())
		return nil
	})
}
