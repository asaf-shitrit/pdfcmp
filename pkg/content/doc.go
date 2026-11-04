// Package content compares the text of two pages by how much of it they
// share, not only by whether it is identical.
//
// Perceptual hashes (package visual) record where blocks of text sit on a
// page and very little of what they say, so a page rewritten in the same
// layout looks nearly identical to them. This package hashes the words
// instead.
//
// # Shingles
//
// A page's text is lowercased, split on whitespace and trimmed of edge
// punctuation, so "1,250.00" stays one token. Each run of k consecutive tokens
// (a w-shingle, k = 3 by default) is hashed to 64 bits. Two pages that share
// most of their shingles share most of their wording, in order.
//
// # MinHash
//
// A MinHash signature keeps, for each of MinHashSize seeded hash functions,
// the smallest hash over the page's shingles. The fraction of slots where two
// signatures agree estimates the Jaccard similarity of the two shingle sets.
// The standard error is sqrt(J(1-J)/MinHashSize), about 0.04 at J = 0.5.
//
// # Why not SimHash
//
// A 64-bit SimHash fingerprint is 128 times smaller, but it counts repeated
// shingles where MinHash sees a set. On templated pages, where every line
// repeats a label and only the values change, the repeats outvote the
// changed values: ten changed values among ten repeated lines read 0.96-0.99
// to SimHash and 0.31-0.41 to MinHash, against an exact Jaccard similarity of
// 0.36-0.38. It is also noisier: unrelated pages read 0 to 0.4. pdfcmp compares
// two files and stores no signatures, so the size buys nothing. The
// comparison is reproduced in bench/simhash_test.go.
//
// A MinHash signature does not reliably see a single changed token on a long
// page: it is an estimate. Keep an exact text hash beside it to know whether
// anything changed, and use the signature to know how much.
package content
