package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"ecrmap/pkg/cache"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage local ecrmap discovery cache",
}

var cacheCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clear all local cached snapshots",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := cache.New()
		if err != nil {
			return err
		}
		if err := c.Clear(); err != nil {
			return fmt.Errorf("failed to clear cache: %w", err)
		}
		fmt.Println("Local cache cleared successfully.")
		return nil
	},
}

var cacheDirCmd = &cobra.Command{
	Use:   "dir",
	Short: "Show the local cache directory path",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := cache.New()
		if err != nil {
			return err
		}
		fmt.Println(c.Dir())
		return nil
	},
}

func init() {
	cacheCmd.AddCommand(cacheCleanCmd)
	cacheCmd.AddCommand(cacheDirCmd)
	rootCmd.AddCommand(cacheCmd)
}
