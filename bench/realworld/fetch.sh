#!/bin/bash
# Downloads the documents the benchmark is built from: real revisions of IRS
# forms and arXiv papers, two long documents, and the first 150 MB of one shard
# each of two Hugging Face datasets: pixparse/pdfa-eng-wds (PDFs from Common
# Crawl) and pixparse/idl-wds (scanned Industry Documents Library papers).
# Set HF_TOKEN, or log in with huggingface-cli, for the Hugging Face part.
# DEST, PDFA_SHARD and IDL_SHARD pick other shards: the held-out set is
# DEST=hf2 PDFA_SHARD=0001 IDL_SHARD=00001 ./fetch.sh
set -euo pipefail
cd "$(dirname "$0")"
DEST=${DEST:-hf}
mkdir -p src "$DEST/pdfa" "$DEST/idl"
get() { curl -fsSL -A pdfcmp-bench -o "src/$1" "$2"; }
get f1040_2024.pdf https://www.irs.gov/pub/irs-prior/f1040--2024.pdf
get f1040_2023.pdf https://www.irs.gov/pub/irs-prior/f1040--2023.pdf
get fw9_2018.pdf https://www.irs.gov/pub/irs-prior/fw9--2018.pdf
get fw9_2024.pdf https://www.irs.gov/pub/irs-prior/fw9--2024.pdf
get p17_2023.pdf https://www.irs.gov/pub/irs-prior/p17--2023.pdf
get attn_v1.pdf https://arxiv.org/pdf/1706.03762v1
get attn_v7.pdf https://arxiv.org/pdf/1706.03762v7
get bert_v1.pdf https://arxiv.org/pdf/1810.04805v1
get bert_v2.pdf https://arxiv.org/pdf/1810.04805v2
get resnet_v1.pdf https://arxiv.org/pdf/1512.03385v1
get nist80063.pdf https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-63-3.pdf
get pdfref.pdf https://opensource.adobe.com/dc-acrobat-sdk-docs/pdfstandards/pdfreference1.0.pdf

token=${HF_TOKEN:-$(cat ~/.cache/huggingface/token 2>/dev/null || true)}
shard() {
  curl -fsSL ${token:+-H "Authorization: Bearer $token"} -r 0-157286399 -o "$DEST/$1.part" \
    "https://huggingface.co/datasets/$2"
  # The range cuts the last file short; tar extracts every whole one before it.
  (cd "$DEST/$1" && tar -xf "../$1.part" '*.pdf' 2>/dev/null || true)
  rm "$DEST/$1.part"
}
shard pdfa "pixparse/pdfa-eng-wds/resolve/main/pdfa-eng-train-${PDFA_SHARD:-0000}.tar"
shard idl "pixparse/idl-wds/resolve/main/idl-train-${IDL_SHARD:-00000}.tar"
echo "pdfa: $(ls "$DEST/pdfa" | wc -l) PDFs, idl: $(ls "$DEST/idl" | wc -l) PDFs"
