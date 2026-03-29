#!/usr/bin/env node

// Script to test on-chain quote for IDRT/USDT on Polygon
// This will help us understand if the issue is on-chain or in backend logic

const { ethers } = require("ethers");

// Polygon mainnet RPC
const RPC_URL = "https://polygon-mainnet.g.alchemy.com/v2/K9PzwLloeXxcOuFEx_fgR";
const provider = new ethers.JsonRpcProvider(RPC_URL);

// Token addresses
const IDRT = "0x554cd6bdD03214b10AafA3e0D4D42De0C5D2937b";
const USDT = "0xc2132D05D31c914a87C6611C10748AEb04B58e8F";
const USDC = "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359";

// Quoter V2 address on Polygon
const QUOTER_V2 = "0x61fFE014bA17989E743c5F6cB21bF9697530B21e";

// Quoter V2 ABI (exactInputSingle)
const QUOTER_ABI = [
  "function quoteExactInputSingle((address tokenIn, address tokenOut, uint256 amountIn, uint24 fee, uint160 sqrtPriceLimitX96)) external returns (uint256 amountOut, uint160 sqrtPriceX96, uint32 initializedTicksCrossed, uint256 gasEstimate)"
];

const quoter = new ethers.Contract(QUOTER_V2, QUOTER_ABI, provider);

// Test amounts (in atomic units with 6 decimals)
const TEST_AMOUNTS = [
  { idrt: "1000000", desc: "1 IDRT" },
  { idrt: "1000000000", desc: "1,000 IDRT" },
  { idrt: "50000000000", desc: "50,000 IDRT" },
  { idrt: "130439810082", desc: "130,439.81 IDRT (problematic amount)" }
];

// Fee tiers to test
const FEE_TIERS = [
  { fee: 100, name: "0.01%" },
  { fee: 500, name: "0.05%" },
  { fee: 3000, name: "0.3%" },
  { fee: 10000, name: "1%" }
];

async function testQuote() {
  console.log("=== Testing On-Chain Quotes for IDRT/USDT on Polygon ===\n");
  
  for (const testAmount of TEST_AMOUNTS) {
    console.log(`Test Amount: ${testAmount.desc} (${testAmount.idrt} atomic)`);
    console.log("-".repeat(60));
    
    for (const feeTier of FEE_TIERS) {
      try {
        const quote = await quoter.quoteExactInputSingle.staticCall({
          tokenIn: IDRT,
          tokenOut: USDT,
          amountIn: testAmount.idrt,
          fee: feeTier.fee,
          sqrtPriceLimitX96: 0
        });
        
        const amountOut = quote.amountOut || quote[0];
        const rate = parseFloat(amountOut.toString()) / parseFloat(testAmount.idrt);
        
        console.log(`  Fee ${feeTier.name.padEnd(6)}: USDT out = ${amountOut.toString()} (rate: ${rate.toFixed(6)})`);
      } catch (err) {
        console.log(`  Fee ${feeTier.name.padEnd(6)}: FAILED - ${err.shortMessage || err.message}`);
      }
    }
    
    console.log("");
  }
  
  // Test USDT/USDC
  console.log("\n=== Testing On-Chain Quotes for USDT/USDC on Polygon ===\n");
  
  const USDT_AMOUNTS = [
    { usdt: "1000000", desc: "1 USDT" },
    { usdt: "3333333333", desc: "~3,333 USDT (equivalent to 50k IDR)" }
  ];
  
  for (const testAmount of USDT_AMOUNTS) {
    console.log(`Test Amount: ${testAmount.desc} (${testAmount.usdt} atomic)`);
    console.log("-".repeat(60));
    
    for (const feeTier of FEE_TIERS) {
      try {
        const quote = await quoter.quoteExactInputSingle.staticCall({
          tokenIn: USDT,
          tokenOut: USDC,
          amountIn: testAmount.usdt,
          fee: feeTier.fee,
          sqrtPriceLimitX96: 0
        });
        
        const amountOut = quote.amountOut || quote[0];
        const rate = parseFloat(amountOut.toString()) / parseFloat(testAmount.usdt);
        
        console.log(`  Fee ${feeTier.name.padEnd(6)}: USDC out = ${amountOut.toString()} (rate: ${rate.toFixed(6)})`);
      } catch (err) {
        console.log(`  Fee ${feeTier.name.padEnd(6)}: FAILED - ${err.shortMessage || err.message}`);
      }
    }
    
    console.log("");
  }
}

testQuote().catch(console.error);
