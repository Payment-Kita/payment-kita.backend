#!/bin/bash

# Enable debug logging
export LOG_LEVEL=debug

# Partner API credentials
PARTNER_API_KEY="pk_live_1c7bddc755765886797fe99792289742"
PARTNER_SECRET_KEY="sk_live_b8c72071541a77680c0087e22d1ea12d"

# Request details - IDRT to IDRX cross-chain
METHOD="POST"
PATH_URL="/api/v1/create-payment"
BODY='{"chain_id":"eip155:137","selected_token":"0x554cd6bdD03214b10AafA3e0D4D42De0C5D2937b","pricing_type":"invoice_currency","requested_amount":"50000"}'

# Generate timestamp
TIMESTAMP=$(date +%s)

# Generate body hash (SHA256)
BODY_HASH=$(echo -n "$BODY" | openssl dgst -sha256 | awk '{print $2}')

# Create canonical string: timestamp.method.path.bodyHash
CANONICAL="${TIMESTAMP}.${METHOD}.${PATH_URL}.${BODY_HASH}"

# Generate HMAC-SHA256 signature
SIGNATURE=$(echo -n "$CANONICAL" | openssl dgst -sha256 -hmac "$PARTNER_SECRET_KEY" | awk '{print $2}')

echo "=== Testing Create Payment: IDRT (Polygon) → IDRX (Base) ==="
echo "Request: 50,000 IDRX"
echo ""
echo "=== Curl Command ==="
RESPONSE=$(curl -sS -X POST http://localhost:8080/api/v1/create-payment \
  -H "Content-Type: application/json" \
  -H "X-PK-Key: $PARTNER_API_KEY" \
  -H "X-PK-Timestamp: $TIMESTAMP" \
  -H "X-PK-Signature: $SIGNATURE" \
  -d "$BODY")

echo "$RESPONSE" | jq '.'

echo ""
echo "=== Analysis ==="
QUOTED_AMOUNT=$(echo "$RESPONSE" | jq -r '.quoted_token_amount')
INVOICE_AMOUNT=$(echo "$RESPONSE" | jq -r '.invoice_amount')
QUOTE_RATE=$(echo "$RESPONSE" | jq -r '.quote_rate')
QUOTE_SOURCE=$(echo "$RESPONSE" | jq -r '.quote_source')

echo "Invoice Amount: $INVOICE_AMOUNT IDRX"
echo "Quoted Amount: $QUOTED_AMOUNT IDRT"
echo "Quote Rate: $QUOTE_RATE"
echo "Quote Source: $QUOTE_SOURCE"
echo ""

# Calculate expected vs actual
if command -v bc &> /dev/null; then
    EXPECTED_RATE="1.0"
    DEVIATION=$(echo "scale=4; ($QUOTE_RATE - $EXPECTED_RATE) / $EXPECTED_RATE * 100" | bc)
    echo "Expected Rate: ~$EXPECTED_RATE (IDR-backed stablecoins)"
    echo "Deviation: ${DEVIATION}%"
fi
