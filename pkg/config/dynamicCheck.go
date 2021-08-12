package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/qri-io/jsonschema"
	"github.com/thoas/go-funk"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
)



func DynamicCheckContainer(container) (bool, []jsonschema.ValError, error) {
	fmt.println('Hello World! Performing dynamicChecks')
	return true, nil,nil
}

