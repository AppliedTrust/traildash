![Unsupported](https://img.shields.io/badge/development_status-unsupported-red.svg)

## Traildash has been retired - AWS now offers a built-in solution based on the same technology (ElasticSearch and Kibana): https://aws.amazon.com/blogs/aws/cloudwatch-logs-subscription-consumer-elasticsearch-kibana-dashboards/

# Traildash: AWS CloudTrail Dashboard
Traildash is a simple, yet powerful, dashboard for AWS CloudTrail logs.

![CloudTrail Dashboard](/readme_images/traildash_screenshot.png)

## What is AWS CloudTrail?
To quote AWS:
> [AWS CloudTrail](http://aws.amazon.com/cloudtrail/) is a web service that records AWS API calls for your account and delivers log files to you. The recorded information includes the identity of the API caller, the time of the API call, the source IP address of the API caller, the request parameters, and the response elements returned by the AWS service.
AWS charges a few dollars a month for CloudTrail for a typical organization.

## Why use Traildash?
The data in CloudTrail is essential, but it's unfortunately trapped in many tiny JSON files stored in AWS S3.  Traildash grabs those files, stores them in ElasticSearch, and presents a Kibana dashboard so you can analyze recent activity in your AWS account.

#### Answer questions like:
* Who terminated my EC2 instance?
* When was that Route53 entry changed?
* What idiot added 0.0.0.0/0 to the security group?

#### Features
* Customizable Kibana dashboards for your CloudTrail logs
* Easy to setup: under 15 minutes
* Self-contained Kibana 3.1.2 release
* HTTPS server with custom SSL cert/key or optional self-signed cert
* Easy-to-deploy Linux/OSX binaries, or a Docker image
* ElasticSearch proxy ensures your logs are secure and read-only
  * No need to open direct access to your ElasticSearch instance
  * Helps to achieve PCI and HIPAA compliance in the cloud

Configure the Traildash Docker container with a few environment variables, and you're off to the races.

## Quickstart
1. [Setup Traildash in AWS](#setup-traildash-in-aws)
1. Fill in the "XXX" blanks and run with docker:

	```
	docker run -i -d -p 7000:7000 \
		-e "AWS_ACCESS_KEY_ID=XXX" \
		-e "AWS_SECRET_ACCESS_KEY=XXX" \
		-e "AWS_SQS_URL=https://XXX" \
		-e "AWS_REGION=XXX"
		-e "DEBUG=1" \
		-v /home/traildash:/var/lib/elasticsearch/ \
		appliedtrust/traildash
	```
1. Open http://localhost:7000/ in your browser

#### Required Environment Variables:
        AWS_SQS_URL              AWS SQS URL.

#### AWS Credentials
AWS Credentials can be provided by either:

1. IAM roles/profiles (See [Setup Traildash in AWS](#setup-traildash-in-aws))
1. Environment Variables
        AWS_ACCESS_KEY_ID       AWS Key ID.
        AWS_SECRET_ACCESS_KEY   AWS Secret Key.

1. Config file (SDK standard format), ~/.aws/credentials

        [default]
        aws_access_key_id = ACCESS_KEY
        aws_secret_access_key = SECRET_KEY
        region = AWS_REGION


#### Optional Environment Variables:
	AWS_REGION		AWS Region (SQS and S3 regions must match. default: us-east-1).
	ES_URL			ElasticSearch URL (default: http://localhost:9200).
	DEBUG			Enable debugging output.
	SSL_MODE		"off": disable HTTPS and use HTTP (default)
				"custom": use custom key/cert stored stored in ".tdssl/key.pem" and ".tdssl/cert.pem"
				"selfSigned": use key/cert in ".tdssl", generate an self-signed cert if empty

## Using traildash outside Docker
We recommend using the appliedtrust/traildash docker container for convenience, as it includes a bundled ElasticSearch instance.  If you'd like to run your own ElasticSearch instance, or simply don't want to use Docker, it's easy to run from the command-line.  The traildash executable is configured with environment variables rather than CLI flags - here's an example:

#### Example Environment Variables
```
export AWS_ACCESS_KEY_ID=AKIXXX
export AWS_SECRET_ACCESS_KEY=XXX
export AWS_SQS_URL=XXX
export AWS_REGION=us-east-1
export WEB_LISTEN=0.0.0.0:7000
export ES_URL=http://localhost:9200
export DEBUG=1
```

#### Usage:
	traildash
	traildash --version

## How it works
1. AWS CloudTrail creates a new log file, stores it in S3, and notifies an SNS topic.
1. The SNS topic notifes a dedicated SQS queue about the new log file in S3.
1. Traildash polls the SQS queue and downloads new log files from S3.
1. Traildash loads the new log files into a local ElasticSearch instace.
1. Kibana provides beautiful dashboards to view the logs stored in ElasticSearch.
1. Traildash protects access to ElasticSearch, ensuring logs are read-only.

## Setup Traildash in AWS
1. Turn on CloudTrail in each region, telling CloudTrail to create a new S3 bucket and SNS topic: ![CloudTrail setup](/readme_images/CloudTrail_Setup.png)
1. If your Traildash instance will be launched in a different AWS account, you must add a bucket policy to your CloudTrail bucket allowing that account access.
```
{
  "Id": "AllowTraildashAccountAccess",
  "Statement": [
    {
      "Sid": "AllowTraildashBucketAccess",
      "Action": [
        "s3:ListBucket"
      ],
      "Effect": "Allow",
      "Resource": "arn:aws:s3:::<your-bucket-name>",
      "Principal": {
        "AWS": [
          "<TRAILDASH ACCOUNT ID>"
        ]
      }
    },
    {
      "Sid": "AllowTraildashObjectAccess",
      "Action": [
        "s3:GetObject"
      ],
      "Effect": "Allow",
      "Resource": "arn:aws:s3:::<your-bucket-name>/*",
      "Principal": {
        "AWS": [
          "<TRAILDASH ACCOUNT ID>"
        ]
      }
    }
  ]
}
```
1. Switch to SNS in your AWS console to view your SNS topic and edit the topic policy: ![CloudTrail setup](/readme_images/SNS_Edit_Topic_Policy.png)
1. Restrict topic access to only allow SQS subscriptions. If you your Traildash instance is launched in the same AWS account, continue on to the next step. If you need Traildash in an outside account to access this topic, allow subscriptions from the AWS account ID that owns your Traildash SQS queue. (If traildash is running the same account, leave "Only me" checked for subscriptions)
![CloudTrail setup](/readme_images/SNS_Basic_Policy.png)
1. Switch to SQS in your AWS console and create a new SQS queue - okay to stick with default options.
1. With your new SQS queue selected, click Queue Actions and "Subscribe Queue to SNS Topic"
![CloudTrail setup](/readme_images/Traildash_SQS1.png)
1. Enter the ARN of your SNS Topic and click Subscribe. Repeat for each CloudTrail SNS Topic you have created. If you encounter any errors in this step, ensure you have created the correct permissions on each SNS topic. ![CloudTrail setup](/readme_images/SQS_Subscribe_to_Topic.png)
1. Create a managed IAM policy with the following policy document, filing in information from the S3 bucket name and SQS queue ARN from above.
```
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowS3BucketAccess",
      "Effect": "Allow",
      "Action": [
        "s3:GetObject"
      ],
      "Resource": [
        "arn:aws:s3:::[YOUR CLOUDTRAIL S3 BUCKET NAME]/*"
      ]
    },
    {
      "Sid": "AllowSQS",
      "Effect": "Allow",
      "Action": [
        "sqs:DeleteMessage",
        "sqs:ReceiveMessage"
      ],
      "Resource": [
        "[YOUR SQS ARN]"
      ]
    }
  ]
}
```
![CloudTrail setup](/readme_images/IAM_Managed_Policy.png)
1. Create a new EC2 instance role in IAM and attach your Traildash policy to it. *Note: Use of IAM roles is NOT required however it is strongly recommended for security best practice.*
![CloudTrail setup](/readme_images/IAM_Create_Role.png)
![CloudTrail setup](/readme_images/IAM_Role_Review.png)
1. Be sure to select this role when launching your Traildash instance!
![CloudTrail setup](/readme_images/EC2_Select_Role.png)

## Backfilling data
Traildash will only pull in data which is being added after the above has been configured, so if you have logs from before this was configured you will have to backfill that data. To make that easier you can use the `backfill.py` Python script provided to notify Traildash of the older data.

The script relies on the same environment variables mentioned above, but also requires a `AWS_S3_BUCKET` variable with the name of the S3 bucket that holds your CloudTrail files. The script also requires some extra permissions than the user for CloudTrail requires, as it needs to list the files in the S3 bucket and also add items to the SQS queue.

The only dependency outside of Python itself is the AWS library, Boto3. It can be installed by running `pip install boto3`.

## Development

#### Contributing
* Fork the project
* Add your feature
* If you are adding new functionality, document it in README.md
* Add some tests if able.
* Push the branch up to GitHub.
* Send a pull request to the appliedtrust/traildash project.

#### Building
This project uses [glock](https://github.com/robfig/glock) for managing 3rd party dependencies.
You'll need to install glock into your workspace before hacking on traildash.
```
$ git clone <your fork>
$ glock sync github.com/appliedtrust/traildash
$ make
```

To cross-compile, you'll need to follow these steps first:
http://dave.cheney.net/2012/09/08/an-introduction-to-cross-compilation-with-go

## Contributors
* [nmcclain](https://github.com/nmcclain)
* [matthewrkrieger](https://github.com/matthewrkrieger)
* [swindmill](https://github.com/swindmill)
* [atward](https://github.com/atward)
* [Tenzer](https://github.com/Tenzer)

## License
MIT
