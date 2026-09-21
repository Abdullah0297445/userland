package aws

import (
	"encoding/base64"
	"errors"
	"testing"
)

func TestParseReadsTheCodeAndTheOperation(t *testing.T) {
	err := Parse("\nAn error occurred (BucketAlreadyOwnedByYou) when calling the CreateBucket operation: Your previous request to create the named bucket succeeded and you already own it.\n")
	var e *Error
	if !errors.As(err, &e) || e.Code != "BucketAlreadyOwnedByYou" || e.Operation != "CreateBucket" || e.Message == "" {
		t.Fatalf("parsed %#v", err)
	}
	err = Parse("An error occurred (404) when calling the HeadBucket operation: Not Found")
	if code(err) != "404" {
		t.Fatalf("head-bucket: %v", err)
	}
	if !Taken(Parse("An error occurred (BucketAlreadyExists) when calling the CreateBucket operation: x")) || !Taken(Parse("An error occurred (403) when calling the HeadBucket operation: Forbidden")) || Taken(Parse("An error occurred (404) when calling the HeadBucket operation: Not Found")) {
		t.Fatal("Taken")
	}
	if Taken(Parse("An error occurred (AccessDenied) when calling the CreateBucket operation: Access Denied")) || Taken(errors.New("docker: daemon down")) {
		t.Fatal("a refusal that is not another account's bucket is not Taken")
	}
	if !KeyLimit(Parse("An error occurred (LimitExceeded) when calling the CreateAccessKey operation: Cannot exceed quota for AccessKeysPerUser: 2")) {
		t.Fatal("KeyLimit")
	}
	if err := Parse("docker: Error response from daemon: something"); err == nil || code(err) != "" || err.Error() != "docker: Error response from daemon: something" {
		t.Fatalf("plain stderr: %v", err)
	}
	if err := Parse("  "); err == nil || err.Error() == "" {
		t.Fatal("empty stderr still an error")
	}
}

func TestPoliciesMatchTheDecidedDocuments(t *testing.T) {
	plain := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket"],"Resource":"arn:aws:s3:::b"},{"Effect":"Allow","Action":["s3:GetObject","s3:PutObject","s3:AbortMultipartUpload"],"Resource":"arn:aws:s3:::b/*"}]}`
	if got := BucketPolicy("b", false, false); got != plain {
		t.Fatalf("plain:\n%s", got)
	}
	versioned := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket","s3:GetBucketVersioning"],"Resource":"arn:aws:s3:::b"},{"Effect":"Allow","Action":["s3:GetObject","s3:PutObject","s3:AbortMultipartUpload"],"Resource":"arn:aws:s3:::b/*"}]}`
	if got := BucketPolicy("b", false, true); got != versioned {
		t.Fatalf("versioned:\n%s", got)
	}
	delete := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket"],"Resource":"arn:aws:s3:::b"},{"Effect":"Allow","Action":["s3:GetObject","s3:PutObject","s3:AbortMultipartUpload","s3:DeleteObject"],"Resource":"arn:aws:s3:::b/*"}]}`
	if got := BucketPolicy("b", true, false); got != delete {
		t.Fatalf("delete:\n%s", got)
	}
	parameter := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["ssm:GetParameter"],"Resource":"arn:aws:ssm:eu-west-1:123456789012:parameter/userland/fort-key-0a1b2c3d"},{"Effect":"Allow","Action":["kms:Decrypt"],"Resource":"arn:aws:kms:eu-west-1:123456789012:key/k"}]}`
	if got := ParameterPolicy("eu-west-1", "123456789012", "/userland/fort-key-0a1b2c3d", "arn:aws:kms:eu-west-1:123456789012:key/k"); got != parameter {
		t.Fatalf("parameter:\n%s", got)
	}
}

func TestNamesAndTheMasterKey(t *testing.T) {
	if UserFromParameter("/userland/fort-key-0a1b2c3d") != "userland-fort-key-0a1b2c3d" || UserFromParameter("plain") != "plain" {
		t.Fatal("UserFromParameter")
	}
	if Endpoint("eu-west-1") != "https://s3.eu-west-1.amazonaws.com" {
		t.Fatal("Endpoint")
	}
	raw, err := base64.StdEncoding.DecodeString(MasterKey())
	if err != nil || len(raw) != 32 || MasterKey() == MasterKey() {
		t.Fatal("MasterKey")
	}
}
