# YUON API Guide

## 인증

| Method | Path | 설명 |
|--------|------|------|
| `POST` | `/api/v1/auth/signup` | 이메일·비밀번호로 회원 가입 후 JWT 반환 |
| `POST` | `/api/v1/auth/login` | 로그인 후 JWT 반환 |

JWT는 `Authorization: Bearer <token>` 헤더로 전달합니다.

## 헬스체크

| Method | Path | 설명 |
|--------|------|------|
| `GET` | `/api/v1/health` | 기본 헬스 체크 (무인증) |
| `GET` | `/api/v1/system/health` | 시스템 헬스 체크 (무인증) |

## 문서 관리 (모두 JWT 필요)

| Method | Path | 설명 | 예시 응답 요약 |
|--------|------|------|----------------|
| `GET` | `/api/v1/documents` | page/pageSize/q/category로 검색 가능한 문서 목록 (`fileKey`, `fileUrl` 포함) | `{ success: true, data: { documents: [ { id, content, metadata, fileKey, fileUrl, score } ], total, page, pageSize, hasNext } } |
| `POST` | `/api/v1/documents` | JSON 본문으로 단일 문서 생성 | `{ success: true, data: { id, message } } |
| `POST` | `/api/v1/documents/bulk-ingest` | 문서 배열을 한 번에 업로드 |
| `GET` | `/api/v1/documents/{id}` | 단일 문서 조회 | `{ success: true, data: { id, content, metadata, fileKey, fileUrl } } |
| `PUT` | `/api/v1/documents/{id}` | 단일 문서 수정 | `{ success: true, data: { id, message } } |
| `DELETE` | `/api/v1/documents/{id}` | 단일 문서 삭제 | `{ success: true, data: { id, message } } |
| `POST` | `/api/v1/documents/reindex` | `{documentIds:[...]}`로 Qdrant 재색인 | `{ success: true, data: { requested, reindexed, failed } } |
| `GET` | `/api/v1/documents/stats` | 전체 문서 통계 | `{ success: true, data: { totalDocuments, index, lastUpdatedAt } } |
| `POST` | `/api/v1/documents/upload` | `multipart/form-data`로 파일 업로드 → S3 저장 + 텍스트 추출 | `{ success: true, data: { message, id, fileUrl, fileKey, fileName } } |
| `GET` | `/api/v1/documents/{id}/file` | 업로드된 원본 파일 다운로드 |


문서 응답의 `metadata`에는 `fileUrl`, `fileKey`, `filename`, `contentType`, `uploadedAt` 등이 포함되므로 업로드한 파일 목록은 `GET /documents`로 확인할 수 있습니다.

## 벡터/프로젝션

| Method | Path | 설명 |
|--------|------|------|
| `GET` | `/api/v1/documents/{id}/vector` | 특정 문서 임베딩 조회 (`withPayload` 옵션) |
| `POST` | `/api/v1/documents/vectors/query` | `{documentIds?, limit?, offset?, withPayload}`로 벡터 검색 |
| `POST` | `/api/v1/documents/vectors/projection` | 벡터를 2D(PCA)로 투영 |

## WebSocket 챗봇

| Method | Path | 설명 |
|--------|------|------|
| `GET` | `/api/v1/ws` | 이벤트 기반 챗봇 (무인증). 초당 5 `append_message` 제한 |

클라이언트 이벤트: `start_conversation`, `append_message`, `typing`, `end_conversation`  
서버 이벤트: `message_ack`, `stream_chunk`, `stream_end`, `system_notice`, `error`

## Swagger

- UI: `GET /docs`
- OpenAPI: `GET /docs/openapi.yaml`

## Analytics

| Method | Path | 설명 | 예시 응답 |
|--------|------|------|------------|
| `GET` | `/api/v1/analytics/chat` | 최근 챗봇 사용 통계 (top keywords/categories 등) | `{ success: true, data: { totalMessages, topKeywords, topCategories, requestsByHour } }` |
| `GET` | `/api/v1/analytics/needs` | 통계를 바탕으로 LLM이 제안하는 자료 보강 영역 | `{ success: true, data: { analysis } }` |
