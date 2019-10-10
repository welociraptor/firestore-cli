package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/api/iterator"
)

var rootCtx context.Context
var client *firestore.Client
var verbose, emulator bool

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringP("collection", "c", "", "collection path")
	rootCmd.PersistentFlags().String("project", "", "gcp project id")
	rootCmd.PersistentFlags().BoolP("prettyprint", "p", false, "pretty print document json")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose mode")

	cmds := []*cobra.Command{whereCmd, documentsCmd}
	for _, cmd := range cmds {
		cmd.Flags().IntP("limit", "l", 100, "return a maximum of n documents")
		cmd.Flags().Bool("unlimited", false, "return all documents in collection (warning: use with precaution)")
	}

	for _, flag := range []string{"collection", "project", "prettyprint"} {
		err := viper.BindPFlag(flag, rootCmd.PersistentFlags().Lookup(flag))
		if err != nil {
			panic(err)
		}
	}

	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(whereCmd)
	rootCmd.AddCommand(documentsCmd)

	rootCtx = context.Background()

	if os.Getenv("FIRESTORE_EMULATOR_HOST") != "" {
		emulator = true
	}
}

var configNotFound = `Config file not found! Consider creating one.

Possible locations:
-------------------
  ./firestore.yaml
  ~/firestore.yaml
  ~/.config/firestore-cli/firestore-cli.yaml

Example configuration:
----------------------
project: my-awesome-gcp-project
collection: my_documents

You can also use --project and --collection switches to override these settings.
`

func initConfig() {
	home, err := homedir.Dir()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	viper.SetConfigName("firestore-cli")
	viper.AddConfigPath(".")
	viper.AddConfigPath(home)
	viper.AddConfigPath(home + "/.config/firestore-cli")
	err = viper.ReadInConfig()
	if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		fmt.Print(configNotFound)
	} else if err != nil {
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "firestore-cli",
	Short: "(Yet another) command line interface for Google Cloud Firestore",
}

func preRunE(cmd *cobra.Command, args []string) error {
	err := validateRequiredParams()
	if err != nil {
		return errors.Wrap(err, "unable to validate required params")
	}

	verbose, err = cmd.Flags().GetBool("verbose")
	if err != nil {
		return errors.Wrap(err, "unable to get flag \"verbose\"")
	}

	err = initFirestoreClient()
	if err != nil {
		return errors.Wrap(err, "unable to create firestore client")
	}
	return nil
}

func validateRequiredParams() error {
	for _, key := range []string{"project", "collection"} {
		if viper.GetString(key) == "" {
			return fmt.Errorf("%s undefined", key)
		}
	}
	return nil
}

func initFirestoreClient() error {
	var err error
	client, err = firestore.NewClient(rootCtx, viper.GetString("project"))
	return errors.Wrap(err, "unable to create firestore client")
}

var getCmd = &cobra.Command{
	Use:     "get [document id]",
	Short:   "get a document by id",
	Args:    cobra.ExactArgs(1),
	PreRunE: preRunE,
	RunE:    get,
}

func get(_ *cobra.Command, args []string) error {
	documentID := args[0]
	if verbose {
		fmt.Printf("Finding (ProjectID:%s, CollectionPath:%s, ID:%s, Emulator:%t)\n",
			viper.GetString("project"),
			viper.GetString("collection"),
			documentID,
			emulator)
	}
	docRef := collection().Doc(documentID)
	ctx, cancelFunc := context.WithTimeout(rootCtx, 5*time.Second)
	defer cancelFunc()
	docSnap, err := docRef.Get(ctx)
	if err != nil {
		return errors.Wrap(err, "unable to get document")
	}

	jsonString, err := jsonString(docSnap.Data())
	if err != nil {
		return err
	}

	fmt.Println(jsonString)
	return nil
}

func collection() *firestore.CollectionRef {
	collectionPath := viper.GetString("collection")
	return client.Collection(collectionPath)
}

var documentsCmd = &cobra.Command{
	Use:     "documents",
	Short:   "return all documents in a collection",
	PreRunE: preRunE,
	RunE:    documents,
}

func documents(cmd *cobra.Command, _ []string) error {
	ctx, cancelFunc := context.WithTimeout(rootCtx, 5*time.Second)
	defer cancelFunc()
	iter := collection().Documents(ctx)
	defer iter.Stop()
	return iterate(cmd, iter)
}

func iterate(cmd *cobra.Command, iter *firestore.DocumentIterator) error {
	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return errors.Wrap(err, "unable to parse flag \"limit\"")
	}
	unlimited, err := cmd.Flags().GetBool("unlimited")
	if err != nil {
		return errors.Wrap(err, "unable to parse flag \"unlimited\"")
	}
	c := 1
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return errors.Wrap(err, "unable to iterate documents")
		}
		jsonString, err := jsonString(doc.Data())
		if err != nil {
			return err
		}
		fmt.Println(jsonString)
		if !unlimited && c >= limit {
			break
		}
		c++
	}
	return nil
}

func jsonString(docData map[string]interface{}) (string, error) {
	var jsonData []byte
	var err error
	if viper.GetBool("prettyprint") {
		jsonData, err = json.MarshalIndent(docData, "", "  ")
	} else {
		jsonData, err = json.Marshal(docData)
	}
	if err != nil {
		return "", errors.Wrap(err, "unable to marshal document to json")
	}
	return string(jsonData), nil
}

var whereCmd = &cobra.Command{
	Use:   "where [name] [operator] [value]",
	Short: "query for documents",
	Long: `examples:
firestore-cli where correlationId == 22da76b6-95c6-4b8f-8381-a60c65752723`,
	Args:    cobra.ExactArgs(3),
	PreRunE: preRunE,
	RunE:    where,
}

func where(cmd *cobra.Command, args []string) error {
	path := args[0]
	op := args[1]
	var value interface{}
	intValue, err := strconv.ParseInt(args[2], 10, 32)
	if err == nil {
		value = intValue
	} else {
		value = args[2]
	}

	if verbose, err := cmd.Flags().GetBool("verbose"); err == nil && verbose {
		fmt.Printf("Querying (project:%s, collection:%s, query:%s %s %v)\n",
			viper.GetString("project"),
			viper.GetString("collection"),
			path, op, value)
	}
	q := collection().Where(path, op, value)
	ctx, cancelFunc := context.WithTimeout(rootCtx, 5*time.Second)
	defer cancelFunc()
	iter := q.Documents(ctx)
	defer iter.Stop()
	return iterate(cmd, iter)
}
