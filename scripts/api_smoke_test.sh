#!/usr/bin/env bash

set -euo pipefail

if ! command -v curl >/dev/null 2>&1; then
  echo "curl 명령어를 찾을 수 없습니다. 설치 후 다시 실행하세요." >&2
  exit 1
fi

BASE_URL="${1:-${BASE_URL:-http://yuon-api.dsmhs.kr}}"
BASE_URL="${BASE_URL%/}"
API_ROOT="${BASE_URL}/api/v1"
DOC_FILE="${DOC_FILE:-document/sample_document.json}"

print_step() {
  printf "\n=== %s ===\n" "$1"
}

request() {
  local method=$1
  local path=$2
  local body=${3:-}
  local url="${API_ROOT}${path}"
  local tmp
  tmp=$(mktemp)
  local status

  if [[ -n "$body" ]]; then
    status=$(curl -sS -o "$tmp" -w "%{http_code}" \
      -X "$method" \
      -H "Content-Type: application/json" \
      "$url" \
      -d "$body")
  else
    status=$(curl -sS -o "$tmp" -w "%{http_code}" \
      -X "$method" \
      "$url")
  fi

  echo "요청: ${method} ${url}"
  echo "상태: ${status}"
  echo "응답:"
  cat "$tmp"
  echo
  rm -f "$tmp"

  if [[ "$status" -ge 400 ]]; then
    echo "요청이 실패했습니다. 위 응답을 확인하세요." >&2
    exit 1
  fi
}

print_step "1. 헬스 체크"
request "GET" "/health"

print_step "2. 단일 문서 추가"
if [[ -f "$DOC_FILE" ]]; then
  echo "문서 파일 사용: ${DOC_FILE}"
  DOC_PAYLOAD=$(cat "$DOC_FILE")
else
  DOC_ID="doc-$(date +%s)"
  DOC_PAYLOAD=$(cat <<JSON
{
  "id": "${DOC_ID}",
  "content": "API 테스트 문서입니다. ${DOC_ID}",
  "metadata": {
    "source": "api-smoke-test",
    "category": "demo"
  }
}
JSON
)
fi
request "POST" "/documents" "$DOC_PAYLOAD"

print_step "3. 심플 챗 요청"
CHAT_PAYLOAD='{
  "message": "대덕소프트웨어마이스터고등학교는 어떤 학교인가요?"
}'
request "POST" "/chat/simple" "$CHAT_PAYLOAD"

print_step "4. 전체 챗 요청"
FULL_CHAT_PAYLOAD=$(cat <<JSON
{
  "message": "전체 챗 엔드포인트 테스트입니다.",
  "useVectorSearch": true,
  "useFullText": true,
  "topK": 5,
  "history": [
    {"role": "user", "content": "이전 대화 히스토리 샘플"}
  ]
}
JSON
)
request "POST" "/chat" "$FULL_CHAT_PAYLOAD"

echo -e "\n모든 API 호출이 성공적으로 완료되었습니다."
