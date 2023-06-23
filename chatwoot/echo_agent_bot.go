package chatwoot

import (
	"net/http"

	"github.com/Kai-Karren/agent-bot-gateway/utils"
	"github.com/Kai-Karren/chatwoot-golang-client/chatwootclient"
	"github.com/gin-gonic/gin"
)

type EchoAgentBot struct {
	ChatwootClient chatwootclient.ChatwootClient
}

func NewEchoAgentBot(chatwootClient chatwootclient.ChatwootClient) EchoAgentBot {

	return EchoAgentBot{
		ChatwootClient: chatwootClient,
	}

}

func (agentBot *EchoAgentBot) HandleMessages(c *gin.Context) {

	var request Event

	if err := c.BindJSON(&request); err != nil {
		utils.LoggerSugar.Errorf("An error occurred while handling the event from chatwoot %s", err)
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"error": "500", "message": err.Error()})
		return
	}

	utils.LoggerSugar.Infof("Received the request: %v", request)

	if request.Event == "message_created" && request.MessageType == "incoming" {

		utils.LoggerSugar.Infof("Received: %v", request.Content)

		responseFromBot := agentBot.GetResponseFromBot(request.Sender.ID, request.Content)

		agentBot.SendResponseBack(request.Account.ID, request.Conversation.ID, responseFromBot)

	}

}

func (agentBot *EchoAgentBot) GetResponseFromBot(sender int, message string) string {
	return message
}

func (agentBot *EchoAgentBot) SendResponseBack(account int, conversationId int, message string) error {

	_, err := agentBot.ChatwootClient.CreateOutgoingMessage(conversationId, message)

	if err != nil {
		utils.LoggerSugar.Errorf("An error occurred while making the http request. %s", err)
		return err
	}

	return nil

}
