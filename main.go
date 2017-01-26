package main

import (
	"fmt"
	"github.com/fvbock/trie"
	"strings"
	"regexp"
	"errors"
	"github.com/alexmk92/stringutil"
)

// Global connection to be used by the server
var DB = Database{}

type Auction struct {
	Seller string
	Timestamp string
	Items []Item
	itemLine string
	raw string
}

type Item struct {
	Name string
	Price float32
	Quantity int16
	ForSale bool
}

func extractParserInformationFromLine(line string, auction *Auction) error {
	fmt.Println("Attempting to match: ", line)
	reg := regexp.MustCompile(`(?m)^\[(?P<Timestamp>[A-Za-z0-9: ]+)+] (?P<Seller>[A-Za-z]+) auction[s]?, '(?P<Items>.+)'$`)
	matches := reg.FindStringSubmatch(line)
	if len(matches) == 0 {
		return errors.New("No matches found for expression")
	}

	auction.raw = line
	auction.Timestamp = matches[0]
	auction.Seller = matches[1]
	auction.itemLine = matches[2]

	return nil
}

func main() {
	// Initialise DB connections
	fmt.Println("Initialising database connection")
	DB.Open()
	fmt.Println("Connection initialised")

	// Create our trie
	trie := trie.NewTrie()

	// Add trade grammar to the trie so we dont empty buffer on sales
	trie.Add("selling")
	trie.Add("buying")
	trie.Add("wtb")
	trie.Add("wts")

	// Load data into our trie
	fmt.Println("Populating table")
	itemQuery := "SELECT displayName, id FROM items ORDER BY displayName ASC"
	rows, _ := DB.Query(itemQuery)
	if rows != nil {
		for rows.Next() {
			var name string
			var id int64

			rows.Scan(&name, &id)
			if id > 0 && name != "" {
				trie.Add(strings.ToLower(name))
			}
		}
	}

	fmt.Println("Populated")

	appendIfInTrie := func(key string, out *[]string) {
		fmt.Println("Checking if: " + key + " can be output")
		if trie.Has(key) {
			fmt.Println("JUP")
			*out = append(*out, key)
		}
	}

	// Try to find item
	items := []string{}
	line := "wts 10 dose blood of the wolf wurmslayermask of wurms>|$swiftwindearthcallerwurmslayerdagger of marnek 250k today I've got some other cool items, WTB mask of wurmsAle 500p"
	line = "Sneeki auctions, 'BUYING Rogue Epic : D  // SELLING Shiny Brass Idol 700pp PST'"
	line = "Stockmarket auctions, 'WTS Mystic/Imbued Koada`Dal Mithril & Dwarven Cultural Armor Full sets and Individual Pieces available'"
	line = "'WTS Mithril Greaves 750 , Lodizal Shell Boots 1.5 , Runed Lava Pendant 800, Fire Emerald Platinum Ring x2 750pp, Orc Fang Earring x2 400pp'"
	line = "[Mon Jan 09 20:34:30 2017] Kandaar auctions, 'WTS Mithril Greaves 750 , Lodizal Shell Boots 1.5 , Runed Lava Pendant 800, Fire Emerald Platinum Ring x2 750pp, Orc Fang Earring x2 400pp'"

	a := Auction{}
	err := extractParserInformationFromLine(line, &a)
	if err != nil {
		fmt.Println(err.Error())
	} else {
		fmt.Println(a.Seller)
		fmt.Println(a.Timestamp)
		fmt.Println(a.raw)
		fmt.Println(a.itemLine)
	}

	buffer := []byte{}
	selling := true
	skippedChar := []byte{} // use an array so we can check the size

	var prevMatch string = ""
	for i, c := range strings.ToLower(a.itemLine) {
		buffer = append(buffer, byte(c))

		// check for selling
		if stringutil.CaseInsenstiveContains(string(buffer), "wts", "selling") {
			selling = true
			buffer = []byte{}
			prevMatch = ""
			skippedChar = []byte{}
			continue
		}
		// check for buying
		if stringutil.CaseInsenstiveContains(string(buffer), "wtb", "buying", "trading") {
			selling = false
			buffer = []byte{}
			prevMatch = ""
			skippedChar = []byte{}
			continue
		}
		// check if we skipped a letter on the previous iteration and shift the items forward
		// this checks example: wurmslayerale it would fail at wurmslayera we set "a" as the
		// skipped character, extract wurmslayer and the begin to match ale using the "a" char
		// once we append to the buffer we reset the skipped char to avoid prepending on
		// subsequent calls
		if len(skippedChar) > 0 {
			buffer = append(skippedChar, buffer...)
			skippedChar = []byte{}
		}
		// extract the item name
		fmt.Println("Checking if: " + string(buffer) + " is a prefix")
		if trie.HasPrefix(string(buffer)) {
			prevMatch = string(buffer)
			fmt.Println("Has prefix: ", string(buffer))
			if i == len(line)-1 {
				buffer = []byte{}
				appendIfInTrie(prevMatch, &items)
				prevMatch = ""
				skippedChar = []byte{}
			}
		} else if prevMatch != "" {
			fmt.Println("Prev was: ", prevMatch)
			skippedChar = append(skippedChar, byte(c))
			buffer = []byte{}
			appendIfInTrie(prevMatch, &items)
			prevMatch = ""
		} else {
			fmt.Println("chillin..." + string(c))
			fmt.Println(prevMatch)
			prevMatch = ""
			buffer = []byte{}
			skippedChar = []byte{}
		}
	}
	fmt.Println("Buffer is: ", string(buffer))
	fmt.Println("Is sell mode? ", selling)
	fmt.Println("Items is: ", items)
}
