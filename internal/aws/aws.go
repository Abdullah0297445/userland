package aws

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const (
	Image      = "amazon/aws-cli:2.36.49"
	PolicyName = "userland"
	KeyAlias   = "alias/aws/ssm"
)

type Admin struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

type Session struct {
	Admin  Admin
	Region string
}

type Key struct {
	ID     string
	Secret string
}

type Error struct {
	Code      string
	Operation string
	Message   string
}

func (e *Error) Error() string {
	return fmt.Sprintf("aws %s: %s (%s)", e.Operation, e.Message, e.Code)
}

var errorLine = regexp.MustCompile(`An error occurred \(([A-Za-z0-9._-]+)\) when calling the (\w+) operation(?: \([^)]*\))?: (.*)`)

func Parse(stderr string) error {
	text := strings.TrimSpace(stderr)
	if m := errorLine.FindStringSubmatch(text); m != nil {
		return &Error{Code: m[1], Operation: m[2], Message: strings.TrimSpace(m[3])}
	}
	if text == "" {
		text = "the aws command failed and said nothing"
	}
	return errors.New(text)
}

func code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func Taken(err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	if e.Code == "BucketAlreadyExists" {
		return true
	}
	if e.Operation != "HeadBucket" {
		return false
	}
	switch e.Code {
	case "403", "Forbidden", "AccessDenied":
		return true
	}
	return false
}

func KeyLimit(err error) bool {
	return code(err) == "LimitExceeded"
}

func Pull() error {
	if exec.Command("docker", "image", "inspect", Image).Run() == nil {
		return nil
	}
	cmd := exec.Command("docker", "pull", Image)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s Session) call(stdin string, args ...string) (string, error) {
	docker := []string{"run", "--rm", "--interactive", "--env", "AWS_ACCESS_KEY_ID", "--env", "AWS_SECRET_ACCESS_KEY", "--env", "AWS_DEFAULT_REGION", "--env", "AWS_PAGER"}
	environment := []string{
		"AWS_ACCESS_KEY_ID=" + s.Admin.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY=" + s.Admin.SecretAccessKey,
		"AWS_DEFAULT_REGION=" + s.Region,
		"AWS_PAGER=",
	}
	if s.Admin.SessionToken != "" {
		docker = append(docker, "--env", "AWS_SESSION_TOKEN")
		environment = append(environment, "AWS_SESSION_TOKEN="+s.Admin.SessionToken)
	}
	if os.Getenv("AWS_ENDPOINT_URL") != "" {
		docker = append(docker, "--env", "AWS_ENDPOINT_URL")
	}
	var command []string
	if stdin == "" {
		command = append(append(append(docker, Image), args...), "--output", "json")
	} else {
		command = append(append(docker, "--entrypoint", "sh", Image, "-c", `exec aws "$@" --cli-input-json "$(cat)" --output json`, "aws"), args...)
	}
	cmd := exec.Command("docker", command...)
	cmd.Env = append(os.Environ(), environment...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", Parse(stderr.String())
	}
	return out.String(), nil
}

func (s Session) Account() (string, error) {
	out, err := s.call("", "sts", "get-caller-identity")
	if err != nil {
		return "", err
	}
	var v struct{ Account string }
	if err := json.Unmarshal([]byte(out), &v); err != nil || v.Account == "" {
		return "", fmt.Errorf("sts get-caller-identity answered without an account: %s", strings.TrimSpace(out))
	}
	return v.Account, nil
}

func (s Session) HasBucket(name string) (bool, error) {
	_, err := s.call("", "s3api", "head-bucket", "--bucket", name)
	switch code(err) {
	case "404", "NoSuchBucket", "NotFound":
		return false, nil
	}
	return err == nil, err
}

func (s Session) CreateBucket(name string) (bool, error) {
	args := []string{"s3api", "create-bucket", "--bucket", name}
	if s.Region != "us-east-1" {
		args = append(args, "--create-bucket-configuration", "LocationConstraint="+s.Region)
	}
	_, err := s.call("", args...)
	if code(err) == "BucketAlreadyOwnedByYou" {
		return true, nil
	}
	return false, err
}

func (s Session) EnableVersioning(name string) error {
	_, err := s.call("", "s3api", "put-bucket-versioning", "--bucket", name, "--versioning-configuration", "Status=Enabled")
	return err
}

func (s Session) CreateUser(name string) (bool, error) {
	_, err := s.call("", "iam", "create-user", "--user-name", name)
	if code(err) == "EntityAlreadyExists" {
		return true, nil
	}
	return false, err
}

func (s Session) PutUserPolicy(user, document string) error {
	_, err := s.call("", "iam", "put-user-policy", "--user-name", user, "--policy-name", PolicyName, "--policy-document", document)
	return err
}

func (s Session) CreateAccessKey(user string) (Key, error) {
	out, err := s.call("", "iam", "create-access-key", "--user-name", user)
	if err != nil {
		return Key{}, err
	}
	var v struct {
		AccessKey struct {
			AccessKeyId     string
			SecretAccessKey string
		}
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil || v.AccessKey.AccessKeyId == "" {
		return Key{}, fmt.Errorf("iam create-access-key answered without a key: %s", strings.TrimSpace(out))
	}
	return Key{v.AccessKey.AccessKeyId, v.AccessKey.SecretAccessKey}, nil
}

func (s Session) PutParameter(name, value string) (bool, error) {
	document, err := json.Marshal(map[string]string{"Name": name, "Type": "SecureString", "Value": value})
	if err != nil {
		return false, err
	}
	_, err = s.call(string(document), "ssm", "put-parameter")
	if code(err) == "ParameterAlreadyExists" {
		return true, nil
	}
	return false, err
}

func (s Session) ParameterKey() (string, error) {
	out, err := s.call("", "kms", "describe-key", "--key-id", KeyAlias)
	if err != nil {
		return "", err
	}
	var v struct{ KeyMetadata struct{ Arn string } }
	if err := json.Unmarshal([]byte(out), &v); err != nil || v.KeyMetadata.Arn == "" {
		return "", fmt.Errorf("kms describe-key answered without an ARN: %s", strings.TrimSpace(out))
	}
	return v.KeyMetadata.Arn, nil
}

type statement struct {
	Effect   string   `json:"Effect"`
	Action   []string `json:"Action"`
	Resource string   `json:"Resource"`
}

type document struct {
	Version   string      `json:"Version"`
	Statement []statement `json:"Statement"`
}

func (d document) String() string {
	out, _ := json.Marshal(d)
	return string(out)
}

func BucketPolicy(bucket string, delete, versioned bool) string {
	onBucket := []string{"s3:ListBucket"}
	if versioned {
		onBucket = append(onBucket, "s3:GetBucketVersioning")
	}
	onObjects := []string{"s3:GetObject", "s3:PutObject", "s3:AbortMultipartUpload"}
	if delete {
		onObjects = append(onObjects, "s3:DeleteObject")
	}
	return document{"2012-10-17", []statement{
		{"Allow", onBucket, "arn:aws:s3:::" + bucket},
		{"Allow", onObjects, "arn:aws:s3:::" + bucket + "/*"},
	}}.String()
}

func ParameterPolicy(region, account, name, keyARN string) string {
	return document{"2012-10-17", []statement{
		{"Allow", []string{"ssm:GetParameter"}, "arn:aws:ssm:" + region + ":" + account + ":parameter/" + strings.TrimPrefix(name, "/")},
		{"Allow", []string{"kms:Decrypt"}, keyARN},
	}}.String()
}

func UserFromParameter(name string) string {
	return strings.TrimPrefix(strings.ReplaceAll(name, "/", "-"), "-")
}

func Endpoint(region string) string {
	return "https://s3." + region + ".amazonaws.com"
}

func MasterKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}
