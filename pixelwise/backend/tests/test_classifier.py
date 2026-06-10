# tests/test_classifier.py
import numpy as np
from app.classifier import classify_batch


def test_classify_batch_returns_one_per_image():
    images = np.zeros((2, 28, 28), dtype=np.uint8)
    results = classify_batch(images)
    assert len(results) == 2


def test_classify_batch_result_shape():
    images = np.zeros((1, 28, 28), dtype=np.uint8)
    result = classify_batch(images)[0]
    assert "prediction" in result
    assert "confidence" in result
    assert isinstance(result["confidence"], float)
