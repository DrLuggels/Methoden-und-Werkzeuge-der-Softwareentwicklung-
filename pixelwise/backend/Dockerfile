# PixelWise-Backend: FastAPI + scikit-learn. Das vortrainierte Kursmodell
# (models/digit_classifier_v1.pkl aus github.com/schutera/pixelwise-model, v1.0)
# wird ins Image kopiert; es benoetigt scikit-learn 1.9 (siehe requirements.txt).
FROM python:3.12-slim

WORKDIR /app

RUN pip install --no-cache-dir -U pip

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

RUN chmod +x entrypoint.sh

EXPOSE 8000
ENTRYPOINT ["./entrypoint.sh"]
