package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: ssmget PARAMETER")
		os.Exit(2)
	}
	if err := get(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "ssmget: "+err.Error())
		os.Exit(1)
	}
}

func get(name string) error {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	out, err := ssm.NewFromConfig(cfg).GetParameter(ctx, &ssm.GetParameterInput{
		Name:           &name,
		WithDecryption: ptr(true),
	})
	if err != nil {
		var missing *types.ParameterNotFound
		if errors.As(err, &missing) {
			return fmt.Errorf("no parameter named %s in this region", name)
		}
		return err
	}
	if out.Parameter == nil || out.Parameter.Value == nil || *out.Parameter.Value == "" {
		return fmt.Errorf("the parameter %s holds nothing", name)
	}
	fmt.Print(*out.Parameter.Value)
	return nil
}

func ptr[T any](v T) *T { return &v }
