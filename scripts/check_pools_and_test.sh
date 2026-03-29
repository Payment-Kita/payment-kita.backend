#!/bin/bash

# Test script to check registered pools and routes

echo "=== Checking Registered Pools on Polygon ==="
echo ""

echo "1. IDRT → USDT (Direct)"
curl -sS "http://localhost:8080/api/v1/tokens/check-pair?chainId=eip155:137&tokenIn=0x554cd6bdD03214b10AafA3e0D4D42De0C5D2937b&tokenOut=0xc2132D05D31c914a87C6611C10748AEb04B58e8F" | jq '{
  executable: .executable,
  isDirect: .isDirect,
  path: .path,
  swapRouterV3: .swapRouterV3
}'

echo ""
echo "2. USDT → USDC (Direct)"
curl -sS "http://localhost:8080/api/v1/tokens/check-pair?chainId=eip155:137&tokenIn=0xc2132D05D31c914a87C6611C10748AEb04B58e8F&tokenOut=0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359" | jq '{
  executable: .executable,
  isDirect: .isDirect,
  path: .path,
  swapRouterV3: .swapRouterV3
}'

echo ""
echo "3. IDRT → USDC (Multi-hop via USDT)"
curl -sS "http://localhost:8080/api/v1/tokens/check-pair?chainId=eip155:137&tokenIn=0x554cd6bdD03214b10AafA3e0D4D42De0C5D2937b&tokenOut=0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359" | jq '{
  executable: .executable,
  isDirect: .isDirect,
  path: .path,
  swapRouterV3: .swapRouterV3
}'

echo ""
echo "=== Checking Registered Pools on Base ==="
echo ""

echo "4. USDC → IDRX (Direct)"
curl -sS "http://localhost:8080/api/v1/tokens/check-pair?chainId=eip155:8453&tokenIn=0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913&tokenOut=0x18Bc5bcC660cf2B9cE3cd51a404aFe1a0cBD3C22" | jq '{
  executable: .executable,
  isDirect: .isDirect,
  path: .path,
  swapRouterV3: .swapRouterV3
}'

echo ""
echo "=== Full Create Payment Test ==="
echo ""

# Partner API credentials
PARTNER_API_KEY="pk_live_1c7bddc755765886797fe99792289742"
PARTNER_SECRET_KEY="sk_live_b8c72071541a77680c0087e22d1ea12d"

METHOD="POST"
PATH_URL="/api/v1/create-payment"
BODY='{"chain_id":"eip155:137","selected_token":"0x554cd6bdD03214b10AafA3e0D4D42De0C5D2937b","pricing_type":"invoice_currency","requested_amount":"50000"}'

TIMESTAMP=$(date +%s)
BODY_HASH=$(echo -n "$BODY" | openssl dgst -sha256 | awk '{print $2}')
CANONICAL="${TIMESTAMP}.${METHOD}.${PATH_URL}.${BODY_HASH}"
SIGNATURE=$(echo -n "$CANONICAL" | openssl dgst -sha256 -hmac "$PARTNER_SECRET_KEY" | awk '{print $2}')

RESPONSE=$(curl -sS -X POST http://localhost:8080/api/v1/create-payment \
  -H "Content-Type: application/json" \
  -H "X-PK-Key: $PARTNER_API_KEY" \
  -H "X-PK-Timestamp: $TIMESTAMP" \
  -H "X-PK-Signature: $SIGNATURE" \
  -d "$BODY")

echo "$RESPONSE" | jq '{
  invoice_amount: .invoice_amount,
  quoted_token_amount: .quoted_token_amount,
  quote_rate: .quote_rate,
  quote_source: .quote_source,
  route_analysis: "IDRT -> USDT -> USDC -> Bridge -> USDC -> IDRX"
}'

echo ""
echo "=== Expected vs Actual ==="
echo "Expected: ~50,000-52,000 IDRT (with 0-4% fees)"
echo "Actual: $(echo "$RESPONSE" | jq -r '.quoted_token_amount') IDRT"
echo "Rate: $(echo "$RESPONSE" | jq -r '.quote_rate') (should be ~1.0-1.04)"
