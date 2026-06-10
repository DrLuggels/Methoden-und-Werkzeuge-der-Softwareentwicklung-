"""Smoke-Test des Klassifikators (Block 8 des Kursskripts)."""
import numpy as np

from app.classifier import classify_batch


def test_classify_batch_shape():
    images = np.zeros((2, 28, 28), dtype=np.uint8)
    images[0, 10:18, 10:18] = 255
    out = classify_batch(images)
    assert len(out) == 2
    for r in out:
        assert "prediction" in r
        assert "confidence" in r
        assert 0.0 <= r["confidence"] <= 1.0
        assert r["prediction"] in [str(d) for d in range(1, 10)]
