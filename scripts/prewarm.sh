#!/bin/sh
echo 'Waiting for Ollama to be ready...'
sleep 30
echo 'Pre-warming llama3.2 model...'
curl -X POST http://ollama:11434/api/generate \
  -H 'Content-Type: application/json' \
  -d '{"model": "llama3.2", "prompt": "Hello", "keep_alive": -1}'
echo 'Model pre-warmed successfully!'
tail -f /dev/null