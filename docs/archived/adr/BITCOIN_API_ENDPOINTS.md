# Bitcoin API Endpoints for Multi-Endpoint Configuration

This document provides a comprehensive list of reliable Bitcoin API endpoints that you can use for your multi-endpoint BTC configuration.

## 🚀 **Recommended Production Endpoints**

### **Tier 1: High Reliability (99.9%+ uptime)**

```bash
# Primary endpoints - best reliability
export BTC_BLOCKSTREAM_API_URLS="https://blockstream.info/api,https://api.mempool.space,https://bitcoin-mainnet.g.alchemy.com/public"
```

1. **Blockstream (Official)**
   - **Mainnet**: `https://blockstream.info/api`
   - **Testnet**: `https://blockstream.info/testnet/api`
   - **Features**: Original Esplora API, high reliability, comprehensive data
   - **Rate Limits**: Generous for public use
   - **Status**: Official Blockstream service

2. **Mempool.space**
   - **Mainnet**: `https://api.mempool.space`
   - **Testnet**: `https://api.mempool.space/testnet`
   - **Features**: Enhanced Esplora API, excellent performance, real-time mempool data
   - **Rate Limits**: Free without authentication
   - **Status**: Highly recommended, often faster than Blockstream

3. **Alchemy Public**
   - **Mainnet**: `https://bitcoin-mainnet.g.alchemy.com/public`
   - **Features**: Enterprise-grade infrastructure, high performance
   - **Rate Limits**: Public endpoint with fair usage
   - **Status**: Backed by major Web3 infrastructure provider

### **Tier 2: Reliable Free Endpoints**

```bash
# Additional reliable endpoints
export BTC_BLOCKSTREAM_API_URLS="https://blockstream.info/api,https://api.mempool.space,https://bitcoin-rpc.publicnode.com,https://bitcoin-mainnet.public.blastapi.io"
```

4. **PublicNode (Allnodes)**
   - **Mainnet**: `https://bitcoin-rpc.publicnode.com`
   - **Testnet**: `https://bitcoin-testnet-rpc.publicnode.com`
   - **Features**: Professional infrastructure, good reliability
   - **Rate Limits**: Fair usage policy

5. **Blast API**
   - **Mainnet**: `https://bitcoin-mainnet.public.blastapi.io`
   - **Testnet**: `https://bitcoin-testnet.public.blastapi.io`
   - **Features**: MEV protection, no registration required
   - **Rate Limits**: Generous for public use

6. **NOWNodes**
   - **Mainnet**: `https://public-btc.nownodes.io`
   - **Features**: Global infrastructure, good performance
   - **Rate Limits**: 1 req/sec for free tier

## 🏗️ **Configuration Examples**

### **Production Mainnet (Recommended)**
```bash
# High availability with 3 reliable endpoints
export BTC_BLOCKSTREAM_API_URL="https://blockstream.info/api"
export BTC_BLOCKSTREAM_API_URLS="https://blockstream.info/api,https://api.mempool.space,https://bitcoin-mainnet.g.alchemy.com/public"
export APP_ENV="prod"
```

### **Production Mainnet (Maximum Redundancy)**
```bash
# Maximum redundancy with 5 endpoints
export BTC_BLOCKSTREAM_API_URL="https://blockstream.info/api"
export BTC_BLOCKSTREAM_API_URLS="https://blockstream.info/api,https://api.mempool.space,https://bitcoin-mainnet.g.alchemy.com/public,https://bitcoin-rpc.publicnode.com,https://bitcoin-mainnet.public.blastapi.io"
export APP_ENV="prod"
```

### **Testnet Development**
```bash
# Testnet for development
export BTC_BLOCKSTREAM_API_URL="https://blockstream.info/testnet/api"
export BTC_BLOCKSTREAM_API_URLS="https://blockstream.info/testnet/api,https://api.mempool.space/testnet,https://bitcoin-testnet-rpc.publicnode.com"
export APP_ENV="local"
```

### **Local Testing**
```bash
# Single endpoint for simple testing
export BTC_BLOCKSTREAM_API_URL="https://blockstream.info/testnet/api"
export APP_ENV="local"
```

## 📊 **Endpoint Comparison**

| Provider | Reliability | Speed | Rate Limits | Features |
|----------|-------------|-------|-------------|----------|
| Blockstream | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | Generous | Official, Complete API |
| Mempool.space | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | No Auth Required | Enhanced features, Fast |
| Alchemy | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | Fair Usage | Enterprise grade |
| PublicNode | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | Fair Usage | Professional |
| Blast API | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | Generous | MEV protection |
| NOWNodes | ⭐⭐⭐ | ⭐⭐⭐ | 1 req/sec free | Global infrastructure |

## 🔧 **API Compatibility**

All listed endpoints are compatible with the Esplora API format used by Blockstream, which means they support:

- ✅ **Address Balance**: `/address/{address}`
- ✅ **Address Transactions**: `/address/{address}/txs`
- ✅ **UTXOs**: `/address/{address}/utxo`
- ✅ **Fee Estimates**: `/fee-estimates`
- ✅ **Broadcast Transaction**: `/tx` (POST)
- ✅ **Transaction Details**: `/tx/{txid}`

## 🚨 **Important Notes**

### **Rate Limiting**
- Most free endpoints have rate limits
- Your multi-endpoint setup will distribute load automatically
- Failed endpoints are temporarily disabled (circuit breaker)

### **Reliability Tips**
1. **Always use multiple endpoints** - Don't rely on just one
2. **Mix providers** - Use different infrastructure providers
3. **Monitor logs** - Watch for endpoint health messages
4. **Test failover** - Occasionally test with one endpoint down

### **Network Selection**
- **Mainnet**: Use production Bitcoin network
- **Testnet**: Use for development and testing (free test coins)
- **Never mix mainnet and testnet endpoints** in the same configuration

## 🧪 **Testing Your Configuration**

### Quick Test
```bash
# Test your configuration
curl -s "https://api.mempool.space/api/address/bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh" | jq .

# Test fee estimates
curl -s "https://api.mempool.space/api/v1/fees/recommended" | jq .
```

### Load Test
```bash
# Test multiple endpoints
for endpoint in "https://blockstream.info/api" "https://api.mempool.space"; do
    echo "Testing $endpoint"
    curl -s "$endpoint/fee-estimates" | jq . | head -5
    echo "---"
done
```

## 🆘 **Troubleshooting**

### Common Issues:
1. **503 Service Unavailable**: Endpoint overloaded - failover will handle this
2. **429 Too Many Requests**: Rate limited - circuit breaker will pause requests
3. **Timeout**: Network issues - automatic retry with next endpoint

### Debug Commands:
```bash
# Check endpoint health
curl -I https://api.mempool.space/api/blocks/tip/height

# Monitor application logs
tail -f logs/app.log | grep "BTC endpoint"
```

## 📈 **Performance Optimization**

### Recommended Order (Fastest to Slowest):
1. `https://api.mempool.space` - Often fastest
2. `https://blockstream.info/api` - Very reliable
3. `https://bitcoin-mainnet.g.alchemy.com/public` - Enterprise grade
4. Other endpoints as backup

### Geographic Considerations:
- **US/Americas**: Alchemy, Blast API
- **Europe**: Mempool.space, Blockstream
- **Asia**: PublicNode, NOWNodes
- **Global**: All listed endpoints have global CDN

---

**Last Updated**: January 2025
**Compatibility**: Esplora API format (Blockstream compatible)
**Support**: These endpoints are actively maintained and monitored