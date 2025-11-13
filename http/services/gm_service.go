package services

type Result int32

const (
	ResultSuccess       Result = 0
	ResultPlayerOffline Result = 1
	ResultItemNotFound  Result = 2
)

type GmService struct {
}

func NewGmService() *GmService {
	return &GmService{}
}
