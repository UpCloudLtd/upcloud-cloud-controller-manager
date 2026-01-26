package mock

import "context"

type UpCloudClient struct{}

func (u *UpCloudClient) Put(_ context.Context, _ string, body []byte) ([]byte, error) {
	return body, nil
}

func NewUpCloudClient() *UpCloudClient {
	return &UpCloudClient{}
}
