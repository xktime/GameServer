package bucket

import (
	"fmt"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/core/log"
	"io"
	"os"
	"sync"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// OSSConfig 阿里云OSS配置结构体
type OSSConfig struct {
	AccessKeyID     string
	AccessKeySecret string
	Endpoint        string
	BucketName      string
}

// OSSClient 阿里云OSS客户端封装
type OSSClient struct {
	client    *oss.Client
	bucket    *oss.Bucket
	config    OSSConfig
	isInit    bool
	openMutex sync.RWMutex
}

var (
	ossClientInstance *OSSClient
	ossClientMutex    sync.Once
)

// GetOSSClient 获取OSS客户端单例
func GetOSSClient() *OSSClient {
	ossClientMutex.Do(func() {
		ossClientInstance = &OSSClient{}
	})
	return ossClientInstance
}

// Init 初始化OSS客户端
func (oc *OSSClient) Init(config OSSConfig) error {
	oc.openMutex.Lock()
	defer oc.openMutex.Unlock()

	oc.config = config

	// 创建OSS客户端
	client, err := oss.New(config.Endpoint, config.AccessKeyID, config.AccessKeySecret)
	if err != nil {
		log.Error("创建OSS客户端失败: %v", err)
		return err
	}
	oc.client = client

	// 尝试获取指定的Bucket
	bucket, err := client.Bucket(config.BucketName)
	if err != nil {
		log.Error("获取指定Bucket失败: %v", err)
		return err
	}
	oc.bucket = bucket
	oc.isInit = true

	log.Release("OSS客户端初始化成功: %s", config.BucketName)
	return nil
}

func (oc *OSSClient) GetBucket() {
	lsRes, err := oc.client.ListBuckets()
	if err != nil {
		// HandleError(err)
	}

	for _, bucket := range lsRes.Buckets {
		fmt.Println("Buckets:", bucket.Name)
	}
}

func (oc *OSSClient) CreateBucket(bucketName string) error {
	err := oc.client.CreateBucket(bucketName)
	if err != nil {
		// HandleError(err)
	}
	return err
}

func (oc *OSSClient) DeleteBucket(bucketName string) error {
	err := oc.client.DeleteBucket(bucketName)
	if err != nil {
		// HandleError(err)
	}
	return err
}

func (oc *OSSClient) UploadFile(objectKey, filePath string) error {
	// 首先检查客户端是否初始化
	if !oc.isInit || oc.client == nil {
		err := fmt.Errorf("OSS客户端未初始化")
		log.Error(err.Error())
		return err
	}

	// 获取Bucket
	bucket := oc.bucket

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		err = fmt.Errorf("本地文件不存在: %s", filePath)
		log.Error(err.Error())
		return err
	}

	// 上传文件
	err := bucket.PutObjectFromFile(objectKey, filePath)
	if err != nil {
		log.Error("上传文件失败: %v", err)
		return err
	}

	log.Release("文件上传成功: %s -> %s", filePath, objectKey)
	return nil
}

func (oc *OSSClient) DownloadObject(objectKey, filePath string) error {
	// 首先检查客户端是否初始化
	if !oc.isInit || oc.client == nil {
		err := fmt.Errorf("OSS客户端未初始化")
		log.Error(err.Error())
		return err
	}

	// 获取Bucket
	bucket := oc.bucket

	// 检查OSS上对象是否存在
	exists, err := bucket.IsObjectExist(objectKey)
	if err != nil {
		log.Error("检查对象是否存在失败: %v", err)
		return err
	}
	if !exists {
		err = fmt.Errorf("OSS对象不存在: %s", objectKey)
		log.Error(err.Error())
		return err
	}

	// 下载文件
	err = bucket.GetObjectToFile(objectKey, filePath)
	if err != nil {
		log.Error("下载文件失败: %v", err)
		return err
	}

	log.Release("文件下载成功: %s -> %s", objectKey, filePath)
	return nil
}

func (oc *OSSClient) DeleteObject(objectKey string) error {
	err := oc.bucket.DeleteObject(objectKey)
	if err != nil {
		log.Error("DeleteObject err: %v", err)
		return err
	}
	return err
}

func (oc *OSSClient) GenerateUploadUrl(t message.UploadType, fileName string) (string, error) {
	options := make([]oss.Option, 0)
	var objectKey string
	switch t {
	case message.UploadType_UploadType_Avatar:
		objectKey = "avatar/" + fileName
		options = append(options, oss.ContentType("image/*"))
	default:
		log.Error("C2S_GetUploadUrlHandler: 未知的上传类型")
		return "", fmt.Errorf("未知的上传类型")
	}
	url, err := oc.bucket.SignURL(objectKey, oss.HTTPPut, 60, options...)
	if err != nil {
		log.Error("ListObjects err: %v", err)
		return "", err
	}
	return url, nil
}

func (oc *OSSClient) PutObjectWithURL(url string, reader io.Reader) error {
	err := oc.bucket.PutObjectWithURL(url, reader)
	if err != nil {
		log.Error("PutObjectWithURL err: %v", err)
		return err
	}
	return nil
}

func (oc *OSSClient) GetObjects(path string, ossLimit int) ([]oss.ObjectProperties, error) {
	lsRes, err := oc.bucket.ListObjectsV2(oss.Prefix(path), oss.MaxKeys(ossLimit), oss.Delimiter("/"))
	if err != nil {
		log.Error("ListObjects err: %v", err)
		return nil, err
	}
	var result []oss.ObjectProperties
	// 排除第一个文件夹，因为它是根目录
	for _, obj := range lsRes.Objects[1:] {
		if obj.Key == "" {
			continue
		}
		result = append(result, obj)
	}
	return result, nil
}

func (oc *OSSClient) CopyObject(objectKey, destFileName string) (string, error) {
	destFilePath := utils.ExtractAndReplace(objectKey, destFileName)
	result, err := oc.bucket.CopyObject(objectKey, destFilePath)
	if err != nil {
		log.Error("CopyObject err: %v", err)
		return "", err
	}
	return result.XMLName.Space, nil
}
