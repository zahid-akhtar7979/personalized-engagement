#!/usr/bin/env bash
# Download Retailrocket Ecommerce Dataset from Kaggle
# Requires: pip install kaggle, ~/.kaggle/kaggle.json configured
set -e
DATA_DIR="$(dirname "$0")/../fallback-data"
mkdir -p "$DATA_DIR"

echo "Downloading Retailrocket dataset..."
kaggle datasets download -d retailrocket/ecommerce-dataset -p "$DATA_DIR" --unzip

if [ -f "$DATA_DIR/events.csv" ]; then
  echo "Dataset ready at $DATA_DIR/events.csv"
  wc -l "$DATA_DIR/events.csv"
else
  echo "events.csv not found. Check Kaggle credentials."
  exit 1
fi
