package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"khairul169/garage-webui/schema"
	"khairul169/garage-webui/utils"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// Lifecycle manages the object expiry rule of a bucket through the S3 API.
type Lifecycle struct{}

type lifecycleBody struct {
	// Days after which objects expire. 0 (or null) removes the rule.
	Days int32 `json:"days"`
}

type lifecycleResult struct {
	Days *int32 `json:"days"`
}

func (l *Lifecycle) Get(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")

	client, err := getOwnerS3Client(bucket)
	if err != nil {
		utils.ResponseError(w, err)
		return
	}

	res, err := client.GetBucketLifecycleConfiguration(context.Background(), &s3.GetBucketLifecycleConfigurationInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) && ae.ErrorCode() == "NoSuchLifecycleConfiguration" {
			utils.ResponseSuccess(w, lifecycleResult{})
			return
		}
		utils.ResponseError(w, err)
		return
	}

	result := lifecycleResult{}
	for _, rule := range res.Rules {
		if rule.Expiration != nil && rule.Expiration.Days != nil && *rule.Expiration.Days > 0 {
			result.Days = rule.Expiration.Days
			break
		}
	}

	utils.ResponseSuccess(w, result)
}

func (l *Lifecycle) Put(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")

	var body lifecycleBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.ResponseErrorStatus(w, err, http.StatusBadRequest)
		return
	}
	if body.Days < 0 {
		utils.ResponseErrorStatus(w, errors.New("days must be 0 or more"), http.StatusBadRequest)
		return
	}

	client, err := getOwnerS3Client(bucket)
	if err != nil {
		utils.ResponseError(w, err)
		return
	}

	if body.Days == 0 {
		_, err = client.DeleteBucketLifecycle(context.Background(), &s3.DeleteBucketLifecycleInput{
			Bucket: aws.String(bucket),
		})
		if err != nil {
			utils.ResponseError(w, fmt.Errorf("cannot remove lifecycle rule: %w", err))
			return
		}
		utils.ResponseSuccess(w, lifecycleResult{})
		return
	}

	_, err = client.PutBucketLifecycleConfiguration(context.Background(), &s3.PutBucketLifecycleConfigurationInput{
		Bucket: aws.String(bucket),
		LifecycleConfiguration: &types.BucketLifecycleConfiguration{
			Rules: []types.LifecycleRule{
				{
					ID:         aws.String(fmt.Sprintf("expire-%dd", body.Days)),
					Status:     types.ExpirationStatusEnabled,
					Filter:     &types.LifecycleRuleFilterMemberPrefix{Value: ""},
					Expiration: &types.LifecycleExpiration{Days: aws.Int32(body.Days)},
				},
				{
					ID:     aws.String("abort-multipart"),
					Status: types.ExpirationStatusEnabled,
					Filter: &types.LifecycleRuleFilterMemberPrefix{Value: ""},
					AbortIncompleteMultipartUpload: &types.AbortIncompleteMultipartUpload{
						DaysAfterInitiation: aws.Int32(2),
					},
				},
			},
		},
	})
	if err != nil {
		utils.ResponseError(w, fmt.Errorf("cannot set lifecycle rule: %w", err))
		return
	}

	utils.ResponseSuccess(w, lifecycleResult{Days: &body.Days})
}

// getOwnerS3Client builds an S3 client from a key with owner permission on the
// bucket, which Garage requires for bucket-level configuration.
func getOwnerS3Client(bucket string) (*s3.Client, error) {
	cacheKey := fmt.Sprintf("ownerkey:%s", bucket)
	if cached := utils.Cache.Get(cacheKey); cached != nil {
		return newS3Client(cached.(aws.CredentialsProvider)), nil
	}

	body, err := utils.Garage.Fetch("/v2/GetBucketInfo?globalAlias="+bucket, &utils.FetchOptions{})
	if err != nil {
		return nil, err
	}

	var bucketData schema.Bucket
	if err := json.Unmarshal(body, &bucketData); err != nil {
		return nil, err
	}

	for _, k := range bucketData.Keys {
		if !k.Permissions.Owner {
			continue
		}

		body, err := utils.Garage.Fetch(fmt.Sprintf("/v2/GetKeyInfo?id=%s&showSecretKey=true", k.AccessKeyID), &utils.FetchOptions{})
		if err != nil {
			return nil, err
		}

		var key schema.KeyElement
		if err := json.Unmarshal(body, &key); err != nil {
			return nil, err
		}

		creds := credentials.NewStaticCredentialsProvider(key.AccessKeyID, key.SecretAccessKey, "")
		utils.Cache.Set(cacheKey, creds, time.Hour)
		return newS3Client(creds), nil
	}

	return nil, fmt.Errorf("bucket %s has no key with owner permission; allow a key as Owner in Permissions", bucket)
}
