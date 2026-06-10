from sqlalchemy import (Column, Integer, String,
                        Float, DateTime, create_engine)
from sqlalchemy.orm import declarative_base, sessionmaker
from datetime import datetime
import os
from dotenv import load_dotenv

load_dotenv()

Base = declarative_base()

class Prediction(Base):
    __tablename__ = "predictions"
    id = Column(Integer, primary_key=True)
    prediction = Column(String, nullable=False)
    confidence = Column(Float, nullable=False)
    model_version = Column(String, nullable=False)
    created_at = Column(DateTime,
                        default=datetime.utcnow)

# Build the connection string in code from a single plain secret.
# DB_PASSWORD is a literal value with no ${...}, so bash, python-dotenv,
# and systemd's EnvironmentFile all read it identically. Interpolating
# ${DB_PASSWORD} inside .env would break under systemd, which does not
# expand variables in an EnvironmentFile.
DB_PASSWORD = os.getenv("DB_PASSWORD")
# Container-Anpassung: DB-Host aus der Umgebung (Compose-Service "db");
# Default bleibt "localhost", sodass das VM-Setup des Kurses unveraendert laeuft.
DB_HOST = os.getenv("DB_HOST", "localhost")
DATABASE_URL = f"postgresql://pixelwise:{DB_PASSWORD}@{DB_HOST}/pixelwise"
engine = create_engine(DATABASE_URL)
SessionLocal = sessionmaker(bind=engine)
