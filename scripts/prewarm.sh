#!/bin/sh

echo 'Waiting for Ollama to be ready...'
sleep 30

# Функция для проверки доступности Ollama
check_ollama() {
    curl -s http://ollama:11434/api/tags > /dev/null
    return $?
}

# Ждем полной готовности Ollama
echo 'Checking Ollama connection...'
until check_ollama; do
    echo 'Ollama not ready yet, waiting...'
    sleep 5
done

echo 'Ollama is ready!'

# Проверяем наличие модели llama3.2
echo 'Checking if llama3.2 model exists...'
MODEL_EXISTS=$(curl -s http://ollama:11434/api/tags | grep -c '"name":"llama3.2:latest"')

if [ "$MODEL_EXISTS" -eq "0" ]; then
    echo 'Model llama3.2 not found. Pulling model...'
    curl -X POST http://ollama:11434/api/pull \
        -H 'Content-Type: application/json' \
        -d '{"name": "llama3.2"}'
    
    # Проверяем успешность загрузки
    if [ $? -eq 0 ]; then
        echo 'Model pulled successfully!'
    else
        echo 'Failed to pull model!'
        exit 1
    fi
else
    echo 'Model llama3.2 already exists!'
fi

echo 'Pre-warming llama3.2 model...'
curl -X POST http://ollama:11434/api/generate \
    -H 'Content-Type: application/json' \
    -d '{"model": "llama3.2", "prompt": "Hello", "keep_alive": -1}'

if [ $? -eq 0 ]; then
    echo 'Model pre-warmed successfully!'
else
    echo 'Failed to pre-warm model!'
    exit 1
fi

# Keep container running
tail -f /dev/null