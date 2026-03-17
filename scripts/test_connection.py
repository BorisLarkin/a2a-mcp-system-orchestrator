import requests
import json
import time

def test_sentence_encoder():
    """Тест Sentence-BERT энкодера"""
    try:
        response = requests.post(
            "http://localhost:8001/embed",
            json={"texts": ["Тестовое предложение для кодирования."]},
            timeout=10
        )
        if response.status_code == 200:
            data = response.json()
            print("✅ Sentence-Encoder работает")
            print(f"   Размер эмбеддинга: {len(data['embeddings'][0])}")
        else:
            print(f"❌ Sentence-Encoder ответил с кодом: {response.status_code}")
    except Exception as e:
        print(f"❌ Sentence-Encoder ошибка: {e}")

def test_classifier():
    """Тест классификатора"""
    try:
        response = requests.post(
            "http://localhost:8002/classify",
            json={
                "text": "У меня не работает интернет, помогите пожалуйста",
                "categories": ["техническая", "биллинг", "жалоба", "общий_вопрос", "срочное"]
            },
            timeout=10
        )
        if response.status_code == 200:
            result = response.json()
            print("✅ Classifier работает")
            print(f"   Предсказанный класс: {result['predicted_class']}")
            print(f"   Уверенность: {result['confidence']:.3f}")
        else:
            print(f"❌ Classifier ответил с кодом: {response.status_code}")
    except Exception as e:
        print(f"❌ Classifier ошибка: {e}")

def test_qdrant():
    """Тест Qdrant"""
    try:
        response = requests.get("http://localhost:6333/", timeout=5)
        if response.status_code == 200:
            print("✅ Qdrant работает")
        else:
            print(f"❌ Qdrant ответил с кодом: {response.status_code}")
    except Exception as e:
        print(f"❌ Qdrant ошибка: {e}")

def wait_for_services():
    """Ожидание запуска сервисов"""
    print("⏳ Ожидаем запуск сервисов...")
    time.sleep(10)  # Даем время на запуск

if __name__ == "__main__":
    print("🧪 Тестирование нейросетевой инфраструктуры...\n")
    
    # Даем сервисам время на запуск
    wait_for_services()
    
    test_sentence_encoder()
    test_classifier() 
    test_qdrant()
    
    print("\n🎯 Тестирование завершено!")