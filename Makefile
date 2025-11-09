.PHONY: help build run clean test docker-build docker-up docker-down dev fmt lint

APP_NAME=yuon
BINARY_NAME=server
DOCKER_IMAGE=$(APP_NAME)-server
BUILD_DIR=bin

help:
	@echo "사용 가능한 명령어:"
	@echo "  make build        - 애플리케이션 빌드"
	@echo "  make run          - 애플리케이션 실행"
	@echo "  make dev          - 개발 모드로 실행 (hot reload)"
	@echo "  make clean        - 빌드 파일 정리"
	@echo "  make test         - 테스트 실행"
	@echo "  make fmt          - 코드 포맷팅"
	@echo "  make lint         - 코드 린팅"
	@echo "  make docker-build - Docker 이미지 빌드"
	@echo "  make docker-up    - Docker Compose로 실행"
	@echo "  make docker-down  - Docker Compose 종료"

build:
	@echo "빌드 중..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/server
	@echo "빌드 완료: $(BUILD_DIR)/$(BINARY_NAME)"

run: build
	@echo "서버 실행 중..."
	@./$(BUILD_DIR)/$(BINARY_NAME)

dev:
	@echo "개발 모드로 실행 중..."
	@go run ./cmd/server/main.go

clean:
	@echo "빌드 파일 정리 중..."
	@rm -rf $(BUILD_DIR)
	@go clean
	@echo "정리 완료"

test:
	@echo "테스트 실행 중..."
	@go test -v -cover ./...

test-coverage:
	@echo "테스트 커버리지 생성 중..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "커버리지 리포트: coverage.html"

fmt:
	@echo "코드 포맷팅 중..."
	@go fmt ./...
	@echo "포맷팅 완료"

lint:
	@echo "코드 린팅 중..."
	@golangci-lint run ./...

install-tools:
	@echo "개발 도구 설치 중..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/cosmtrek/air@latest
	@echo "도구 설치 완료"

docker-build:
	@echo "Docker 이미지 빌드 중..."
	@docker build -t $(DOCKER_IMAGE):latest .
	@echo "Docker 이미지 빌드 완료"

docker-up:
	@echo "Docker Compose 실행 중..."
	@docker-compose -f docker_compose.yaml up -d
	@echo "서비스 시작 완료"

docker-down:
	@echo "Docker Compose 종료 중..."
	@docker-compose -f docker_compose.yaml down
	@echo "서비스 종료 완료"

docker-logs:
	@docker-compose -f docker_compose.yaml logs -f

migrate-up:
	@echo "데이터베이스 마이그레이션 적용 중..."
	@# TODO: 마이그레이션 도구 설정 필요

migrate-down:
	@echo "데이터베이스 마이그레이션 롤백 중..."
	@# TODO: 마이그레이션 도구 설정 필요

deps:
	@echo "의존성 다운로드 중..."
	@go mod download
	@go mod tidy
	@echo "의존성 설치 완료"

vendor:
	@echo "Vendor 생성 중..."
	@go mod vendor
	@echo "Vendor 생성 완료"
