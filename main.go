package main

import (
	"bytes"
	"fmt"
	"html/template"
	"net/smtp"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"

	"github.com/playwright-community/playwright-go"
)

const DebugEnv string = "BPG_BOT_DEBUG"

const TeslaModelYInventorySiteUrlEnv string = "BPG_TESLA_MY_INVENTORY_SITEURL"

// IT inventory, new model y, price from low to high, location everywhere
const TeslaModelYInventorySiteUrlDefault string = "https://www.tesla.com/it_IT/inventory/new/my?arrangeby=plh&zip=20161&range=0"

const TeslaModel3InventorySiteUrlEnv string = "BPG_TESLA_M3_INVENTORY_SITEURL"

// IT inventory, new model 3, price from low to high, location everywhere
const TeslaModel3InventorySiteUrlDefault string = "https://www.tesla.com/it_IT/inventory/new/m3?arrangeby=plh&zip=20161&range=0"

const InventoryArticlesXpathEnv string = "BPG_INVENTORY_ARTICLES_XPATH"
const InventoryArticlesXpathDefault string = "//html/body/div[1]/div/div[1]/main/div/article"

const ReferenceModelYPriceEnv string = "BPG_MODELY_REFERENCE_PRICE"
const ReferenceModelYPriceDefault string = "36500"

const ReferenceModel3PriceEnv string = "BPG_MODEL3_REFERENCE_PRICE"
const ReferenceModel3PriceDefault string = "36500"

const MailSmtpUrlEnv string = "BPG_BOT_SMTP_URL"
const MailSmtpUrlDefault string = "smtp://user:password@hostname:port?starttls=true"

const MailgunUserEnv string = "MAILGUN_SMTP_LOGIN"
const MailgunUserDefault string = "mailgunuser"

const MailgunPasswordEnv string = "MAILGUN_SMTP_PASSWORD"
const MailgunPasswordDefault string = "mailgunpasswd"

const MailgunSmtpPortEnv string = "MAILGUN_SMTP_PORT"
const MailgunSmtpPortDefault string = "587"

const MailgunSmtpHostnameEnv string = "MAILGUN_SMTP_SERVER"
const MailgunSmtpHostnameDefault string = "mailgunhostname"

const MailFromEnv string = "BPG_BOT_MAIL_FROM"
const MailFromDefault string = "bot@example.com"

const MailToEnv string = "BPG_BOT_MAIL_TO"
const MailToDefault string = "tmp@example.com"

const mailMessageTemplate string = "" +
	"To: {{.MailTo}}\r\n" +
	"From: {{.MailFrom}}\r\n" +
	"Subject: Tesla Price Alert - {{.CarModel}} new article !\r\n" +
	"\r\n" +
	"{{.CarModel}} market price {{.CarPrice}} EUR is lower than your reference price {{.ReferencePrice}} EUR ! Click this link to see the available items: {{.SiteUrl}}"

type MailInfo struct {
	MailTo         string
	MailFrom       string
	CarModel       string
	CarPrice       string
	ReferencePrice string
	SiteUrl        string
}

type CarModelType int

const (
	CarModelType_MY CarModelType = 1
	CarModelType_M3 CarModelType = 2
)

func (s CarModelType) String() string {
	switch s {
	case 1:
		return "model_y"
	case 2:
		return "model_3"
	}
	return "unknown"
}

type carModelPriceEntry struct {
	date  time.Time
	price float64
}

func main() {
	fmt.Println("Model Y")
	scrapeCarModelPrice(CarModelType_MY)

	fmt.Println("Model 3")
	scrapeCarModelPrice(CarModelType_M3)
}

func scrapeCarModelPrice(carModelType CarModelType) {
	pw, err := playwright.Run()
	if err != nil {
		fmt.Printf("[main.go:bot-playwright-go] could not start playwright: %v\n", err)
	}

	// Chromium does not work with Tesla site (it's a bot-proof web site), using firefox (or Webkit)
	browser, err := pw.Firefox.Launch()
	if err != nil {
		fmt.Printf("[main.go:bot-playwright-go] could not launch browser: %v\n", err)
	}

	page, err := browser.NewPage()
	if err != nil {
		fmt.Printf("[main.go:bot-playwright-go] could not create page: %v\n", err)
	}

	var siteUrl string
	if carModelType == CarModelType_MY {
		siteUrl = getEnv(TeslaModelYInventorySiteUrlEnv, TeslaModelYInventorySiteUrlDefault)
	} else if carModelType == CarModelType_M3 {
		siteUrl = getEnv(TeslaModel3InventorySiteUrlEnv, TeslaModel3InventorySiteUrlDefault)
	}

	fmt.Println("Loading URL:", siteUrl)

	if _, err = page.Goto(siteUrl); err != nil {
		fmt.Printf("[main.go:bot-playwright-go] could not goto: %v\n", err)
	}

	// Important: wait a bit while the initial pop-up is dismissed before loading the page content - quick and dirty but it would be better to use a "wait until" pattern see https://stackoverflow.com/a/76622997
	// <article class="result card" data-id="234_92624100cf47408fe4b9f97c3fc14800-search-result-container">
	// XPATH: //html/body/div[1]/div/div[1]/main/div/article[@class='result card']
	fmt.Println("Waiting for web page content to be loaded ...")
	time.Sleep(5 * time.Second)

	htmlstring, err := page.Content()
	if err != nil {
		fmt.Printf("[main.go:bot-playwright-go] could not load page content: %v\n", err)
	}

	// DEBUG: dumping the resulting html
	// f, err := os.OpenFile("/tmp/page.html", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	// if err != nil {
	// 	panic(err)
	// }
	// f.WriteString(htmlstring)
	// defer f.Close()

	htmldoc, err := htmlquery.Parse(strings.NewReader(htmlstring))
	if err != nil {
		fmt.Printf("[main.go:bot-playwright-go] could not parse page content: %v\n", err)
	}

	firstCarModelPriceEntry := scrapeCarModelPriceEntry(htmldoc, getEnv(InventoryArticlesXpathEnv, InventoryArticlesXpathDefault))

	var refCarModelPrice float64
	if carModelType == CarModelType_MY {
		refCarModelPrice, _ = strconv.ParseFloat(getEnv(ReferenceModelYPriceEnv, ReferenceModelYPriceDefault), 64)
	} else if carModelType == CarModelType_M3 {
		refCarModelPrice, _ = strconv.ParseFloat(getEnv(ReferenceModel3PriceEnv, ReferenceModel3PriceDefault), 64)
	}

	// for keep track of the data over time - saving it to db
	carModelPriceInsert(carModelType, firstCarModelPriceEntry.date, firstCarModelPriceEntry.price)

	if isEnvGreaterThan(DebugEnv, 1000) {
		fmt.Printf("Price[%v] Target price[%v]\n", firstCarModelPriceEntry.price, refCarModelPrice)
	}

	// 	check if there is a car model with a price lower than the reference price
	if firstCarModelPriceEntry.price <= refCarModelPrice {
		sendMail(carModelType, firstCarModelPriceEntry, refCarModelPrice, siteUrl)
	} else {
		fmt.Printf("[main.go:bot-playwright-go] no lower car price found\n")
	}

	// close playwright
	if err = browser.Close(); err != nil {
		fmt.Printf("[main.go:bot-playwright-go] could not close browser: %v\n", err)
	}

	if err = pw.Stop(); err != nil {
		fmt.Printf("[main.go:bot-playwright-go] could not stop Playwright: %v\n", err)
	}
}

func scrapeCarModelPriceEntry(htmldoc *html.Node, xpath string) carModelPriceEntry {

	firstArticle, err := htmlquery.Query(htmldoc, xpath)
	if err != nil {
		fmt.Printf("[main.go:bot-playwright-go] could not find html node: %v\n", err)
	}

	if isEnvGreaterThan(DebugEnv, 1000) {
		fmt.Printf("First article: %v\n", htmlquery.InnerText(firstArticle))
	}

	// <span class="result-purchase-price tds-text--h4">39.545&nbsp;€</span>
	// /html/body/div[1]/div/div[1]/main/div/article[1]/section[1]/div[2]/div[1]/div/span[1]

	articlePrice, err := htmlquery.Query(firstArticle, "//section[1]/div[2]/div[1]/div/span[1]")
	if err != nil {
		fmt.Printf("[main.go:bot-playwright-go] could not find html node: %v\n", err)
	}

	if isEnvGreaterThan(DebugEnv, 1000) {
		fmt.Printf("Price: %v\n", htmlquery.InnerText(articlePrice))
	}

	// Price: 39.545 €
	price := strings.Replace(strings.Replace(strings.Replace(string(htmlquery.InnerText(articlePrice)), ".", "", -1), "€", "", -1), "\u00a0", "", -1)

	var priceEntry carModelPriceEntry
	priceEntry.date = time.Now()
	priceEntry.price, _ = strconv.ParseFloat(price, 64)

	return priceEntry
}

func sendMail(energyCostEntryType CarModelType, lowestCarModelPriceEntry carModelPriceEntry, refCarModelPrice float64, siteUrl string) {
	// by default it's expected to work with Mailgun add-on, but it's possible to override the smtp URL with the MailSmtpUrlEnv

	var mailSmtpUrl string
	if isEnv(MailSmtpUrlEnv) {
		fmt.Printf("Using external SMTP service to send emails\n")

		mailSmtpUrl = getEnv(MailSmtpUrlEnv, MailSmtpUrlDefault)
	} else {
		fmt.Printf("Using Mailgun to send emails\n")

		mailSmtpUrl = fmt.Sprintf("smtp://%s:%s@%s:%s?starttls=true", getEnv(MailgunUserEnv, MailgunUserDefault), getEnv(MailgunPasswordEnv, MailgunPasswordDefault), getEnv(MailgunSmtpHostnameEnv, MailgunSmtpHostnameDefault), getEnv(MailgunSmtpPortEnv, MailgunSmtpPortDefault))
	}

	u, err := url.Parse(mailSmtpUrl)
	if err != nil {
		fmt.Printf("Invalid SMTP URL: %v\n", err)
	} else {
		// https://docs.cloudmailin.com/outbound/examples/send_email_with_golang/

		// hostname is used by PlainAuth to validate the TLS certificate.
		hostname := u.Hostname()
		passwd, _ := u.User.Password()
		auth := smtp.PlainAuth("", u.User.Username(), passwd, hostname)

		mailInfo := MailInfo{getEnv(MailToEnv, MailToDefault), getEnv(MailFromEnv, MailFromDefault), strings.ToUpper(energyCostEntryType.String()), fmt.Sprint(lowestCarModelPriceEntry.price), fmt.Sprint(refCarModelPrice), fmt.Sprint(siteUrl)}

		tmpl, err := template.New("mailTemplate").Parse(mailMessageTemplate)
		if err != nil {
			panic(err)
		}

		var mailMsg bytes.Buffer
		err = tmpl.Execute(&mailMsg, mailInfo)
		if err != nil {
			panic(err)
		}

		fmt.Printf("[main.go:bot-playwright-go] market price [%v EUR] is lower than current target price [%v EUR], sending email alert ...\n", lowestCarModelPriceEntry.price, refCarModelPrice)
		err = smtp.SendMail(hostname+":"+u.Port(), auth, mailInfo.MailFrom, []string{mailInfo.MailTo}, mailMsg.Bytes())
		if err != nil {
			fmt.Printf("Error sending email: %v\n", err)
		}
	}
}

func isEnv(key string) bool {
	if _, ok := os.LookupEnv(key); ok {
		return true
	}
	return false
}

func isEnvGreaterThan(key string, val int64) bool {
	if v, ok := os.LookupEnv(key); ok {
		intval, _ := strconv.ParseInt(string(v), 10, 64)

		return intval > val
	}

	return false
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
