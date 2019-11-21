package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// licenseCmd represents the license command
var licenseCmd = &cobra.Command{
	Use:   "license",
	Short: "Display license information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf(`%[1]s is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

%[1]s is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with %[1]s. If not, see <http://www.gnu.org/licenses/>.
`, strings.Title(RootCmd.Use))
	},
}

func init() {
	RootCmd.AddCommand(licenseCmd)
}
