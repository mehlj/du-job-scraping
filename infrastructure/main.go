package main

import (
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ssm"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lambda"
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

		// IAM role for lambda
		role, err := iam.NewRole(ctx, "duscraper-exec-role", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(`{
				"Version": "2012-10-17",
				"Statement": [{
					"Sid": "",
					"Effect": "Allow",
					"Principal": {
						"Service": "lambda.amazonaws.com"
					},
					"Action": "sts:AssumeRole"
				}]
			}`),
		})
		if err != nil {
			return err
		}

		// attach policy
		policyJSON := pulumi.Sprintf(`{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Effect": "Allow",
					"Action": [
						"logs:CreateLogGroup",
						"logs:CreateLogStream",
						"logs:PutLogEvents"
					],
					"Resource": "arn:aws:logs:*:*:*"
				},
				{
					"Effect": "Allow",
					"Action": [
						"ssm:GetParameter"
					],
					"Resource": "arn:aws:ssm:us-east-1:252267185844:parameter/du-job-scraping/s3BucketName"
				},
				{
					"Effect": "Allow",
					"Action": [
						"s3:GetObject",
						"s3:PutObject"
					],
					"Resource": "arn:aws:s3:::%s/jobs.json"
				},
				{
					"Effect": "Allow",
					"Action": [
						"s3:ListBucket"
					],
					"Resource": "arn:aws:s3:::%s"
				}
			]
		}`, bucket.ID(), bucket.ID())

		lambdaPolicy, err := iam.NewRolePolicy(ctx, "lambda-policy", &iam.RolePolicyArgs{
			Role:   role.Name,
			Policy: policyJSON,
		})
		if err != nil {
			return err
		}

		// set arguments for constructing the function resource
		// NOTE: use `make build` to generate this zip and binary
		args := &lambda.FunctionArgs{
			Handler: pulumi.String("bootstrap"), // name of compiled go binary
			Role:    role.Arn,
			Runtime: pulumi.String("provided.al2023"),     // amazon linux runtime (no golang exists)
			Code:    pulumi.NewFileArchive("scraper.zip"), // name of zip containing binary
			Timeout: pulumi.Int(60),
		}

		// create the lambda using the args
		function, err := lambda.NewFunction(
			ctx,
			"du-job-scraping",
			args,
			pulumi.DependsOn([]pulumi.Resource{lambdaPolicy}),
		)
		if err != nil {
			return err
		}

		// export the lambda ARN
		ctx.Export("lambda", function.Arn)

		return nil
	})
}
