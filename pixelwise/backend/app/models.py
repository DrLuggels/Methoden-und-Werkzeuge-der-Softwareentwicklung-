"""SQLAlchemy-Datenmodell und DB-Verbindung (Block 6 des Kursskripts).

Anpassung fuer Container: Die Verbindungs-URL wird aus einzelnen
Umgebungsvariablen gebaut (DB_HOST etc.), damit der DB-Host der
Compose-Service-Name (``db``) sein kann statt ``localhost``.
"""
import os
from datetime import datetime

from dotenv import load_dotenv
from sqlalchemy import Column, DateTime, Float, Integer, String, create_engine
from sqlalchemy.orm import declarative_base, sessionmaker

load_dotenv()

Base = declarative_base()


class Prediction(Base):
    __tablename__ = "predictions"
    id = Column(Integer, primary_key=True)
    prediction = Column(String, nullable=False)
    confidence = Column(Float, nullable=False)
    model_version = Column(String, nullable=False)
    client_ip = Column(String)
    created_at = Column(DateTime, default=datetime.utcnow)


DB_HOST = os.getenv("DB_HOST", "localhost")
DB_USER = os.getenv("DB_USER", "pixelwise")
DB_NAME = os.getenv("DB_NAME", "pixelwise")
DB_PASSWORD = os.getenv("DB_PASSWORD", "")

DATABASE_URL = (
    f"postgresql+psycopg2://{DB_USER}:{DB_PASSWORD}@{DB_HOST}/{DB_NAME}"
)
engine = create_engine(DATABASE_URL, pool_pre_ping=True)
SessionLocal = sessionmaker(bind=engine)
