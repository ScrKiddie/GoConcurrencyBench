#!/bin/bash

# script untuk download dataset dari imagine kisi.pcz.pl
# dataset ini berisi foto dari 3 kamera nikon berbeda

UPLOAD_DIR="storage/uploads"

mkdir -p "$UPLOAD_DIR"

echo "=== Download Dataset IMAGINE ==="
echo "Target: $UPLOAD_DIR"
echo ""

# nikon d5 - 38 gambar
echo "--- Downloading Nikon D5 (38 images) ---"
for i in $(seq 1 38); do
    FILENAME="Nikon_D5_${i}.jpg"
    echo "  [$i/38] $FILENAME"
    curl -k -s -o "$UPLOAD_DIR/$FILENAME" "https://kisi.pcz.pl/imagine/img/Nikon_D5/${i}.jpg"
done
echo ""

# nikon d810 - 31 gambar
echo "--- Downloading Nikon D810 (31 images) ---"
for i in $(seq 1 31); do
    FILENAME="Nikon_D810_${i}.jpg"
    echo "  [$i/31] $FILENAME"
    curl -k -s -o "$UPLOAD_DIR/$FILENAME" "https://kisi.pcz.pl/imagine/img/Nikon_D810/${i}.JPG"
done
echo ""

# nikon z7 - 31 gambar (ambil 31 agar total 100)
echo "--- Downloading Nikon Z7 (31 images) ---"
for i in $(seq 1 31); do
    FILENAME="Nikon_Z7_${i}.jpg"
    echo "  [$i/31] $FILENAME"
    curl -k -s -o "$UPLOAD_DIR/$FILENAME" "https://kisi.pcz.pl/imagine/img/Nikon_Z7/${i}.JPG"
done
echo ""

TOTAL=$(ls -1 "$UPLOAD_DIR"/*.jpg 2>/dev/null | wc -l)
echo "=== Selesai ==="
echo "Total: $TOTAL gambar di $UPLOAD_DIR"
