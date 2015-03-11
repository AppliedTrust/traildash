#!/usr/bin/env python

import json
from os import environ

import boto3


bucket = boto3.resource('s3').Bucket(environ.get('AWS_S3_BUCKET'))
queue = boto3.resource('sqs').Queue(environ.get('AWS_SQS_URL'))


items_queued = 0
for item in bucket.objects.all():
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
