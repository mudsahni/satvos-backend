package dynamodb

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"satvos/internal/config"
	"satvos/internal/domain"
	"satvos/internal/port"
)

// emailConfigItem mirrors the DynamoDB item schema.
type emailConfigItem struct {
	TenantSlug     string   `dynamodbav:"tenant_slug"`
	ServiceAPIKey  string   `dynamodbav:"service_api_key"`
	Enabled        bool     `dynamodbav:"enabled"`
	AllowedSenders []string `dynamodbav:"allowed_senders"`
	APIBaseURL     string   `dynamodbav:"api_base_url"`
	InboundAddress string   `dynamodbav:"inbound_address,omitempty"`
}

// EmailConfigRepo implements port.TenantEmailConfigRepository backed by DynamoDB.
type EmailConfigRepo struct {
	client    *dynamodb.Client
	tableName string
}

// NewEmailConfigRepo creates a new DynamoDB-backed email config repository.
func NewEmailConfigRepo(cfg *config.DynamoDBConfig) (*EmailConfigRepo, error) {
	var opts []func(*awsconfig.LoadOptions) error
	opts = append(opts, awsconfig.WithRegion(cfg.Region))

	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("loading aws config: %w", err)
	}

	var ddbOpts []func(*dynamodb.Options)
	if cfg.Endpoint != "" {
		ddbOpts = append(ddbOpts, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		})
	}

	client := dynamodb.NewFromConfig(awsCfg, ddbOpts...)
	return &EmailConfigRepo{
		client:    client,
		tableName: cfg.TableName,
	}, nil
}

func (r *EmailConfigRepo) Get(ctx context.Context, tenantSlug string) (*port.TenantEmailConfig, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"tenant_slug": &types.AttributeValueMemberS{Value: tenantSlug},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("dynamodb GetItem: %w", err)
	}
	if out.Item == nil {
		return nil, domain.ErrNotFound
	}

	var item emailConfigItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return nil, fmt.Errorf("unmarshaling dynamodb item: %w", err)
	}

	return &port.TenantEmailConfig{
		TenantSlug:     item.TenantSlug,
		ServiceAPIKey:  item.ServiceAPIKey,
		Enabled:        item.Enabled,
		AllowedSenders: item.AllowedSenders,
		APIBaseURL:     item.APIBaseURL,
		InboundAddress: item.InboundAddress,
	}, nil
}

func (r *EmailConfigRepo) Put(ctx context.Context, item *port.TenantEmailConfig) error {
	ddbItem := emailConfigItem{
		TenantSlug:     item.TenantSlug,
		ServiceAPIKey:  item.ServiceAPIKey,
		Enabled:        item.Enabled,
		AllowedSenders: item.AllowedSenders,
		APIBaseURL:     item.APIBaseURL,
		InboundAddress: item.InboundAddress,
	}

	av, err := attributevalue.MarshalMap(ddbItem)
	if err != nil {
		return fmt.Errorf("marshaling dynamodb item: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("dynamodb PutItem: %w", err)
	}
	return nil
}

func (r *EmailConfigRepo) UpdateConfig(ctx context.Context, tenantSlug string, enabled bool, allowedSenders []string) error {
	sendersAV, err := attributevalue.MarshalList(allowedSenders)
	if err != nil {
		return fmt.Errorf("marshaling allowed_senders: %w", err)
	}

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"tenant_slug": &types.AttributeValueMemberS{Value: tenantSlug},
		},
		UpdateExpression:    aws.String("SET enabled = :e, allowed_senders = :s"),
		ConditionExpression: aws.String("attribute_exists(tenant_slug)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":e": &types.AttributeValueMemberBOOL{Value: enabled},
			":s": &types.AttributeValueMemberL{Value: sendersAV},
		},
	})
	if err != nil {
		return fmt.Errorf("dynamodb UpdateItem: %w", err)
	}
	return nil
}
