// Copyright (c) 2019 ACSONE SA/NV
// Distributed under the MIT License (http://opensource.org/licenses/MIT)

package main

import (
	"os"

	"github.com/acsone/kwkhtmltopdf/client/go/kwkhtmlclient"
)

func main() {
	serverURL, err := kwkhtmlclient.ServerURLFromEnv()
	if err == nil {
		err = kwkhtmlclient.Run(serverURL, "/pdf", os.Args[1:], os.Stdout)
	}
	if err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(-1)
	}
}
