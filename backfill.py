#!/usr/bin/env python

####################
# Neccesary Environment Variables:
# AWS_S3_BUCKET - bucket name to search in
# AWS_SQS_URL - SQS queue to send messages to
# AWS_REGION - AWS region to work in. Must be the same for bucket and sqs
#
# Optional parameter
# <prefix> - pass an optional S3 prefix as first parameter
####################


import json
import sys
from os import environ

import boto3


if not all([environ.get('AWS_S3_BUCKET'), environ.get('AWS_SQS_URL')]):
    print('You have to specify the AWS_S3_BUCKET and AWS_SQS_URL environment variables.')
    print('Check the "Backfilling data" section of the README file for more info.')
    exit(1)


bucket = boto3.resource('s3',region_name=environ.get('AWS_REGION')).Bucket(environ.get('AWS_S3_BUCKET'))
queue = boto3.resource('sqs',region_name=environ.get('AWS_REGION')).Queue(environ.get('AWS_SQS_URL'))

if len(sys.argv) >= 2:
    print('S3 prefix ' + sys.argv[1])
    items = bucket.objects.filter(Prefix=sys.argv[1])
else:
    items = bucket.objects.all()

items_queued = 0
for item in items:
    if not item.key.endswith('.json.gz'):
        continue

    queue.send_message(
        MessageBody=json.dumps({
            'Message': json.dumps({
                's3Bucket': environ.get('AWS_S3_BUCKET'),
                's3ObjectKey': [item.key]
            })
        })
    )
    items_queued += 1

print('Done! {} items were backfilled'.format(items_queued))
