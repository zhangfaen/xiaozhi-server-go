#!/bin/bash
# 测试火山引擎 ASR WebSocket

CONN_ID="test-$(date +%s)"
echo "Connect ID: $CONN_ID"

# 发送 JSON 请求并接收响应
websocat -t 5 "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_nostream" \
  --header "X-Api-App-Key: 8833371206" \
  --header "X-Api-Access-Key: -1yBtIJ3p6l3ApQC6zZhhH_ZEkaPV5o-" \
  --header "X-Api-Resource-Id: volc.bigasr.sauc.duration" \
  --header "X-Api-Connect-Id: $CONN_ID" \
  --message-json '{"user":{"uid":"test"},"audio":{"format":"mp3","rate":24000,"bits":16,"channel":1,"language":"zh-CN"},"request":{"model_name":"bigmodel","result_type":"single"}}'
