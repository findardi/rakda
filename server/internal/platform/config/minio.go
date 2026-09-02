package config

type MinioConfig struct {
	Endpoint          string
	AccessKey         string
	SecretKey         string
	BucketName        string
	SslMode           bool
	RequireEncryption bool
}

func LoadMinioConfig() MinioConfig {
	return MinioConfig{
		Endpoint:          GetEnv("MINIO_ENDPOINT", "localhost:9000"),
		AccessKey:         GetEnv("MINIO_ACCESS_KEY", "miniouser"),
		SecretKey:         GetEnv("MINIO_SECRET_KEY", "miniopassword"),
		BucketName:        GetEnv("MINIO_BUCKET", "rakda-file"),
		SslMode:           GetEnv("MINIO_SSL_MODE", "false") == "true",
		RequireEncryption: GetEnv("MINIO_REQUIRE_ENCRYPTION", "false") == "true",
	}
}
