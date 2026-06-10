"""Legt das Datenbankschema an (Block 6 des Kursskripts)."""
from app.models import Base, engine

if __name__ == "__main__":
    Base.metadata.create_all(engine)
    print("Schema angelegt.")
