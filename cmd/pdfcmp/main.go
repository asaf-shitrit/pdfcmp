package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/asafshitrit/pdfcmp/pkg/compare"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	verbose bool
	quiet   bool
	format  string

	// Compare flags
	mode      string
	dpi       int
	workers   int
	threshold float64
	hashType  string
	sampling  string
	maxMemory int
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "pdfcmp",
		Short: "Lightning-fast PDF comparison engine with visual perception",
		Long: `pdfcmp is a high-performance comparison tool and library that uses a
multi-stage pipeline (byte-wise, structural, and visual) to detect
identical or similar PDF documents with extreme speed.`,
	}

	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppress all output except result")
	rootCmd.PersistentFlags().StringVarP(&format, "format", "f", "text", "Output format: text, json")

	// Compare command
	compareCmd := &cobra.Command{
		Use:   "compare <file1.pdf> <file2.pdf>",
		Short: "Compare two PDF files",
		Long: `Compare two PDF files using a multi-layer pipeline:

  Layer 1: Byte-wise comparison (xxHash64)
  Layer 2: Thumbnail comparison (first/last pages at 72 DPI)
  Layer 3: Strategic sampling (sqrt(n)+2 pages)
  Layer 4: Full comparison (all pages if needed)`,
		Args: cobra.ExactArgs(2),
		RunE: runCompare,
	}

	compareCmd.Flags().StringVarP(&mode, "mode", "m", "auto", "Comparison mode: auto, byte, visual, full")
	compareCmd.Flags().IntVar(&dpi, "dpi", 150, "Rendering DPI for visual comparison")
	compareCmd.Flags().IntVarP(&workers, "workers", "w", runtime.NumCPU(), "Number of worker goroutines")
	compareCmd.Flags().Float64VarP(&threshold, "threshold", "t", 0.95, "Similarity threshold (0.0 to 1.0)")
	compareCmd.Flags().StringVar(&hashType, "hash", "both", "Hash type: dhash, phash, both")
	compareCmd.Flags().StringVar(&sampling, "sampling", "auto", "Sampling strategy: auto, all, strategic, thumbnail")
	compareCmd.Flags().IntVar(&maxMemory, "memory", 512, "Max memory in MB")

	// Quick command
	quickCmd := &cobra.Command{
		Use:   "quick <file1.pdf> <file2.pdf>",
		Short: "Quick byte-wise comparison only",
		Long:  `Perform only byte-wise comparison using xxHash64. This is the fastest mode.`,
		Args:  cobra.ExactArgs(2),
		RunE:  runQuick,
	}

	// Version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("pdfcmp v0.1.0")
		},
	}

	rootCmd.AddCommand(compareCmd, quickCmd, versionCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runCompare(cmd *cobra.Command, args []string) error {
	pdf1, pdf2 := args[0], args[1]

	// Validate files exist and are accessible
	if err := validateFile(pdf1); err != nil {
		return err
	}
	if err := validateFile(pdf2); err != nil {
		return err
	}

	// Parse options
	opts := []compare.Option{
		compare.WithDPI(dpi),
		compare.WithWorkers(workers),
		compare.WithThreshold(threshold),
		compare.WithMaxMemory(maxMemory),
	}

	// Parse mode
	switch mode {
	case "auto":
		opts = append(opts, compare.WithMode(compare.ModeAuto))
	case "byte":
		opts = append(opts, compare.WithMode(compare.ModeByte))
	case "visual":
		opts = append(opts, compare.WithMode(compare.ModeVisual))
	case "full":
		opts = append(opts, compare.WithMode(compare.ModeFull))
	default:
		return fmt.Errorf("invalid mode: %s", mode)
	}

	// Parse hash type
	switch hashType {
	case "dhash":
		opts = append(opts, compare.WithHashType(compare.HashDHash))
	case "phash":
		opts = append(opts, compare.WithHashType(compare.HashPHash))
	case "both":
		opts = append(opts, compare.WithHashType(compare.HashBoth))
	default:
		return fmt.Errorf("invalid hash type: %s", hashType)
	}

	// Parse sampling
	switch sampling {
	case "auto":
		opts = append(opts, compare.WithSampling(compare.SamplingAuto))
	case "all":
		opts = append(opts, compare.WithSampling(compare.SamplingAll))
	case "strategic":
		opts = append(opts, compare.WithSampling(compare.SamplingStrategic))
	case "thumbnail":
		opts = append(opts, compare.WithSampling(compare.SamplingThumbnailOnly))
	default:
		return fmt.Errorf("invalid sampling strategy: %s", sampling)
	}

	// Add progress callback if verbose
	if verbose && !quiet {
		opts = append(opts, compare.WithProgress(func(p compare.Progress) {
			fmt.Fprintf(os.Stderr, "\r%s: %d/%d (%.0f%%)", p.Stage, p.CurrentPage, p.TotalPages, p.Percent*100)
		}))
	}

	// Create comparer
	cmp, err := compare.New(opts...)
	if err != nil {
		return fmt.Errorf("failed to create comparer: %w", err)
	}
	defer cmp.Close()

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Run comparison
	if !quiet {
		fmt.Fprintf(os.Stderr, "Comparing %s and %s...\n", pdf1, pdf2)
	}

	result, err := cmp.Compare(ctx, pdf1, pdf2)
	if err != nil {
		return fmt.Errorf("comparison failed: %w", err)
	}

	// Clear progress line
	if verbose && !quiet {
		fmt.Fprintln(os.Stderr)
	}

	// Output result
	return outputResult(result)
}

func runQuick(cmd *cobra.Command, args []string) error {
	pdf1, pdf2 := args[0], args[1]

	// Validate files exist and are accessible
	if err := validateFile(pdf1); err != nil {
		return err
	}
	if err := validateFile(pdf2); err != nil {
		return err
	}

	identical, err := compare.Quick(pdf1, pdf2)
	if err != nil {
		return fmt.Errorf("comparison failed: %w", err)
	}

	if format == "json" {
		output := map[string]interface{}{
			"identical": identical,
			"file1":     pdf1,
			"file2":     pdf2,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	if identical {
		fmt.Println("IDENTICAL: Files are byte-for-byte identical")
	} else {
		fmt.Println("DIFFERENT: Files differ")
	}

	return nil
}

func outputResult(result *compare.Result) error {
	switch format {
	case "json":
		jsonStr, err := result.JSON()
		if err != nil {
			return err
		}
		fmt.Println(jsonStr)
	default:
		fmt.Print(result.Summary())
	}

	// Exit with code 1 if files are different
	if !result.Identical && result.SimilarityScore < threshold {
		os.Exit(1)
	}

	return nil
}

// validateFile checks if a file exists and is accessible, providing specific error messages
func validateFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", path)
		}
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied: cannot read %s", path)
		}
		return fmt.Errorf("cannot access file %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("expected a file but got a directory: %s", path)
	}
	return nil
}
