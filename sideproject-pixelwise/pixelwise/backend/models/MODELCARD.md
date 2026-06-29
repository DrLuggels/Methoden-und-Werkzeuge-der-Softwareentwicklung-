# MODELCARD.md: digit_classifier_v1

## Training Data
MNIST digits 1-9, ~63k train / ~7k test, public domain.
Class 0 withheld intentionally so a v2 release can add it later.
Pixel values normalised from [0, 255] to [0, 1] before training.

## Capabilities
Predict handwritten digits 1-9.
Expected accuracy: ~92% (LogisticRegression baseline).

## Known Failures
6/9 confusion, 3/8 confusion, messy or rotated digits.
Class 0 is out of distribution; predictions on a hand-drawn 0 are
arbitrary and should not be trusted.

## Intended Use
28x28 canvas drawings, greyscale, pixel values in [0, 255] (uint8) or
[0, 1] (float). Out of scope: photos, non-digit characters, rotated or
heavily skewed inputs.

## Pipeline Contract
The artefact is an `sklearn.pipeline.Pipeline` with two named steps:

- `binarize`: `Binarizer(threshold=0.5)` — idempotent on already-binarised
  {0.0, 1.0} inputs, so callers that pre-binarise at uint8 threshold 128
  pass through unchanged.
- `clf`: `LogisticRegression(max_iter=1000, solver="lbfgs")`.

Inputs:
- shape `(N, 784)` (flattened 28x28),
- dtype `float32` or `float64`,
- value range either {0.0, 1.0} (pre-binarised) or [0, 1] (the pipeline
  binarises at 0.5).

`pipeline.classes_` returns `["1", "2", "3", "4", "5", "6", "7", "8", "9"]`.
`pipeline.n_features_in_` returns `784`.

## Reproducibility
Trained by `train.py` in this repo with `random_state=42` and a 90/10
stratified split. Rerun:

    python -m venv .venv
    source .venv/bin/activate
    pip install -r requirements.txt
    python train.py

MNIST downloads automatically via `sklearn.datasets.fetch_openml` and
is cached in `~/scikit_learn_data/`.

## Version
v1.0 — initial release, 9-class.
