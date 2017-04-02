package main
import (
	"github.com/Syfaro/telegram-bot-api"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
	"os"
)

const (
	Help   = "/help"
	Payer  = "/payer"
	Add    = "/add"
	Remove = "/remove"
	Status = "/status"
	Solve  = "/solve"
	Clear  = "/clear"
)

type (
	BillItem struct {
		Amount  float64  `bson:"amount"`
		Members []string `bson:"members"`
	}

	Bill struct {
		Id         int64      `bson:"id"`
		Accountant string     `bson:"accountant"`
		Items      []BillItem `bson:"items"`
	}
)

var host     = os.Getenv("BOT_DB_HOST")
var database = os.Getenv("BOT_DB_NAME")
var username = os.Getenv("BOT_DB_USERNAME")
var password = os.Getenv("BOT_DB_PASSWORD")
var token    = os.Getenv("BOT_TG_TOKEN")

var commandRegexp  = regexp.MustCompile("^/[a-zA-Z_0-9]+")
var usernameRegexp = regexp.MustCompile("@[a-zA-Z_0-9]+")
var amountRegexp   = regexp.MustCompile("[0-9]+[.]?[0-9]{0,2}")
var numberRegexp   = regexp.MustCompile("[0-9]+")

var debug    = false
var debugBot = false

func main() {
	// Connect to DB
	mongoDialInfo := &mgo.DialInfo{
		Addrs:    []string{host},
		Timeout:  60 * time.Second,
		Database: database,
		Username: username,
		Password: password,
	}
	mongoSession, err := mgo.DialWithInfo(mongoDialInfo)
	if err != nil {
		logFatalf("Error connecting to DB: %s", err)
	}
	mongoSession.SetMode(mgo.Monotonic, true)
	collection:= mongoSession.DB(database).C("bill")

	// Connect to Telegram
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		logFatalf("Error connecting to Telegram API: %s", err)
	}
	bot.Debug = debugBot
	logInfof("Connected to bot @%s", bot.Self.UserName)

	// Subscribe to updates
	var config tgbotapi.UpdateConfig = tgbotapi.NewUpdate(0)
	config.Timeout = 60
	updates, err := bot.GetUpdatesChan(config)

	for update := range updates {
		var response string
		if update.Message == nil {
			continue
		}

		// Get message info
		username := update.Message.From.UserName
		chatId := update.Message.Chat.ID
		text := update.Message.Text
		logInfof("Received message from user @%s in chat %d: '%s'",
			username, chatId, text)

		// Find a bill for this chat in DB
		var bill *Bill
		err = collection.Find(selector(chatId)).One(&bill)
		if err != nil && err != mgo.ErrNotFound {
			sendMessage(chatId, bot, "There seem to be problems with this bot\n" +
				"Please send a message to @onobrod about it")
			logFatalf("Error finding a bill in DB: %s", err)
		}

		if bill == nil {
			bill = &Bill{
				Id: chatId,
			}
			err = collection.Insert(bill)
			if err != nil {
				sendMessage(chatId, bot, "There seem to be problems with this bot\n" +
					"Please send a message to @onobrod about it")
				logFatalf("Error adding a bill to DB: %s", err)
			}
			logDebugf("Added new bill %s", *bill)
		}

		command := commandRegexp.FindString(text)
		logInfof("Processing %s command...", command)

		Processing:
		switch command {
		case Payer:
			username := usernameRegexp.FindString(text)

			bill.Accountant = username
			err = collection.Update(selector(bill.Id), bill)
			if err != nil {
				sendMessage(chatId, bot, "There seem to be problems with this bot\n" +
					"Please send a message to @onobrod about it")
				logFatalf("Error updating a bill in DB: %s", err)
			}

			response = fmt.Sprintf("Set %s as accountant", username)
		case Add:
			index := strings.Index(text, " ")
			if index == -1 {
				logDebug("Message parsing failed")
				response = "I cannot parse your message :(\n" +
					"Send */help* to show help info"
				break Processing
			}
			text = text[index+1:]

			usernames := usernameRegexp.FindAllString(text, -1)
			amounts := amountRegexp.FindAllString(text, -1)
			logDebugf("Parsed message: amounts %s, usernames %s", amounts, usernames)

			var total float64
			if amounts == nil {
				total = 0
			} else {
				for _, amount := range amounts {
					a, err := strconv.ParseFloat(amount, 64)
					if err != nil {
						logDebugf("Amount %s parsing failed", amount)
						response = fmt.Sprintf("I cannot parse value %s :(\n" +
							"Send */help* to show help info", amount)
						break Processing
					}
					total += a
				}
			}
			logDebugf("Total amount: %.2f", total)

			item := BillItem{
				Amount: total,
				Members: usernames,
			}

			bill.Items = append(bill.Items, item)
			err = collection.Update(selector(bill.Id), bill)
			if err != nil {
				sendMessage(chatId, bot, "There seem to be problems with this bot\n" +
					"Please send a message to @onobrod about it")
				logFatalf("Error updating a bill in DB: %s", err)
			}

			response = fmt.Sprintf("Added a payment of %.2f", total)
		case Remove:
			index := numberRegexp.FindString(text)
			i, err := strconv.ParseInt(index, 10, 64)
			if err != nil {
				logDebugf("Index %s parsing failed", index)
				response = fmt.Sprintf("I cannot parse value %s :(\n" +
					"Send */help* to show help info", index)
				break Processing
			}
			if i < 1 || i > int64(len(bill.Items)) {
				logDebugf("Index %d is out of range", i)
				response = fmt.Sprintf("Index %d is invalid. Index must be in range [%d, %d]",
					index, 1, len(bill.Items))
				break Processing
			}
			bill.Items = append(bill.Items[:i-1], bill.Items[i:]...)
			err = collection.Update(selector(bill.Id), bill)
			if err != nil {
				sendMessage(chatId, bot, "There seem to be problems with this bot\n" +
					"Please send a message to @onobrod about it")
				logFatalf("Error updating a bill in DB: %s", err)
			}
			response = "Bill item has been removed"
		case Status:
			if len(bill.Items) == 0 {
				response = "Bill is empty"
			} else {
				response = "Bill items:"
			}
			for i, item := range bill.Items {
				response += "\n" +
					fmt.Sprintf("%d. *%.2f* by", i + 1, item.Amount)
				for _, member := range item.Members {
					response += " " + member
				}
			}

			response += "\n\n" + fmt.Sprintf("Default payer is %s", bill.Accountant)
		case Solve:
			response = "Payments:\n"
			sum := make(map[string]float64)
			for _, item := range bill.Items {
				for _, itm := range item.Members {
					sum[itm] = sum[itm] + item.Amount / float64(len(item.Members))
				}
			}

			for member, amount := range sum {
				if member == bill.Accountant {
					continue
				}
				response += "\n" +
					fmt.Sprintf("*%.2f* from %s to %s", amount, member, bill.Accountant)
			}
		case Clear:
			bill.Items = nil
			err = collection.Update(selector(bill.Id), bill)
			if err != nil {
				sendMessage(chatId, bot, "There seem to be problems with this bot\n" +
					"Please send a message to @onobrod about it")
				logFatalf("Error updating a bill in DB: %s", err)
			}
			response = "Bill items have been removed"
		case Help:
			response = "Hi, I'm the Party Accountant Bot \n" +
				"I can help you calculate debts after party \n\n" +
				"You can use following commands: \n" +
				"/help — show help info \n" +
				"/payer — set default payer \n" +
				"/add — add a bill item \n" +
				"/remove — remove a bill item \n" +
				"/status — show list of bill items and default payer \n" +
				"/solve — calculate debts \n" +
				"/clear — clear list of bill items \n\n" // TODO: add examples
		default:
			logDebug("Command is not supported")
			response = "This command is not supported \n" +
				"Send */help* to show all available commands"
		}

		if response != "" {
			sendMessage(chatId, bot, response)
		}
	}
}

func sendMessage(id int64, bot *tgbotapi.BotAPI, text string) {
	msg := tgbotapi.NewMessage(id, text)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
	logInfof("Response has been sent: %s", text)
}

func selector(id interface{}) (interface{}) {
	return bson.M{"id": id}
}

func logInfof(format string, a ...interface{}) {
	logInfo(fmt.Sprintf(format, a...))
}

func logInfo(message string) {
	log.Print("[INFO] " + message)
}

func logDebugf(format string, a ...interface{}) {
	logDebug(fmt.Sprintf(format, a...))
}

func logDebug(message string) {
	if debug {
		log.Print("[DEBUG] " + message)
	}
}

func logFatalf(format string, a ...interface{}) {
	log.Fatalf("[ERROR] " + format, a...)
}
