package voiceflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Kai-Karren/agent-bot-gateway/chatwoot"
	"github.com/Kai-Karren/agent-bot-gateway/utils"
	"github.com/Kai-Karren/chatwoot-golang-client/chatwootclient"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type VoiceflowAgentBot struct {
	VoiceflowClient               VoiceflowClient
	ChatwootClient                chatwootclient.ChatwootClient
	WebWidgetTriggersStartMessage bool
	ErrorMessage                  string
	InboxID                       int
}

func NewVoiceflowAgentBot(chatwootClient chatwootclient.ChatwootClient, inboxID int, voiceflowClient VoiceflowClient, errorMessage string) VoiceflowAgentBot {

	return VoiceflowAgentBot{
		ChatwootClient:                chatwootClient,
		WebWidgetTriggersStartMessage: false,
		InboxID:                       inboxID,
		VoiceflowClient:               voiceflowClient,
		ErrorMessage:                  errorMessage,
	}

}

type VoiceflowClient struct {
	VoiceflowInteractUrl string
	VoiceflowAPIKey      string
}

type VoiceflowResponse struct {
	Type    string                   `json:"type"`
	Payload VoiceflowResponsePayload `json:"payload"`
}

type VoiceflowResponsePayload struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (client *VoiceflowClient) SendMessage(userId int, message string) ([]VoiceflowResponse, error) {

	content := fmt.Sprintf(`{"action": {"type": "text", "payload": "%s"}}`, message)

	utils.Logger.Info(content)

	jsonBody := []byte(content)

	bodyReader := bytes.NewReader(jsonBody)

	url := fmt.Sprintf(client.VoiceflowInteractUrl, userId)

	request, err := http.NewRequest(http.MethodPost, url, bodyReader)

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", client.VoiceflowAPIKey)

	if err != nil {
		utils.LoggerSugar.Errorf("An error occurred while creating the http request. %s", err)
		return []VoiceflowResponse{}, err
	}

	result, err := http.DefaultClient.Do(request)

	if err != nil {
		utils.LoggerSugar.Errorf("An error occurred while making the http request. %s", err)
		return []VoiceflowResponse{}, err
	}

	body, err := io.ReadAll(result.Body)

	if err != nil {
		utils.LoggerSugar.Errorf("client: could not read response body: %s", err)
		return []VoiceflowResponse{}, err
	}

	var voiceflowResponses []VoiceflowResponse

	if err := json.Unmarshal(body, &voiceflowResponses); err != nil {
		utils.Logger.Error("Can not unmarshal JSON")
	}

	utils.Logger.Info("Received the response from Voiceflow", zap.Any("response", voiceflowResponses))

	return voiceflowResponses, nil

}

func (agentBot *VoiceflowAgentBot) HandleMessages(c *gin.Context) {

	var request chatwoot.Event

	if err := c.BindJSON(&request); err != nil {
		utils.Logger.Error(err.Error())
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"error": "500", "message": err.Error()})
		return
	}

	utils.Logger.Info("Received the request: ", zap.Any("request", request))

	if request.Event == "webwidget_triggered" && agentBot.WebWidgetTriggersStartMessage {

		utils.LoggerSugar.Infof("Source Id %v", request.SourceID)

		var conversation_id int

		if request.CurrentConversation.ID != 0 {
			utils.LoggerSugar.Infof("A conversation already exists with id %d. No new conversation is created.", request.CurrentConversation.ID)
			return
		} else {
			con_id, err := agentBot.CreateConversation(request.SourceID)

			if err == nil {
				conversation_id = con_id
			} else {
				utils.Logger.Error(err.Error())
			}
		}

		voiceflowResponses, err := agentBot.GetResponseFromBot(conversation_id, "start")

		if len(voiceflowResponses) == 0 {
			utils.LoggerSugar.Errorf("No response from voiceflow.")
			return
		}

		for _, voiceflowResponse := range voiceflowResponses {

			if voiceflowResponse.Type != "text" {
				continue
			}

			if err != nil {

			} else {
				agentBot.SendResponseBack(conversation_id, voiceflowResponse)
			}

		}

		return

	}

	if request.Event == "message_created" && request.MessageType == "incoming" {

		utils.Logger.Info("Received " + request.Content)

		voiceflowResponses, err := agentBot.GetResponseFromBot(request.Conversation.ID, request.Content)

		if err != nil {

			agentBot.SetErrorLabelAndSendErrorMessageBack(request.Conversation.ID)

		} else {

			for _, voiceflowResponse := range voiceflowResponses {

				if voiceflowResponse.Type != "text" {
					continue
				}

				err = agentBot.SendResponseBack(request.Conversation.ID, voiceflowResponse)

				if err != nil {
					utils.LoggerSugar.Errorf("The message could not be send back to chatwoot. %s", err.Error())
					agentBot.SetErrorLabelAndSendErrorMessageBack(request.Conversation.ID)
				}

				// TODO
				// err = agentBot.ApplyCustomAttributes(request.Conversation.ID, voiceflowResponse)

				// if err != nil {
				// 	utils.LoggerSugar.Errorf("An error occurred while trying to apply the custom attributes. %s", err.Error())
				// }

			}

		}

	}

}

func (agentBot *VoiceflowAgentBot) SetErrorLabelAndSendErrorMessageBack(conversation_id int) {

	agentBot.ChatwootClient.AddLabel(conversation_id, "agent_bot_error")

	agentBot.SendResponseBack(conversation_id, VoiceflowResponse{
		Type: "speak",
		Payload: VoiceflowResponsePayload{
			Type:    "message",
			Message: agentBot.ErrorMessage,
		},
	})
}

func (agentBot *VoiceflowAgentBot) GetResponseFromBot(conversation_id int, message string) ([]VoiceflowResponse, error) {

	return agentBot.VoiceflowClient.SendMessage(conversation_id, message)

}

func (agentBot *VoiceflowAgentBot) CreateConversation(sourceId string) (int, error) {

	response, err := agentBot.ChatwootClient.CreateNewConversation(chatwootclient.CreateNewConversationRequest{
		SourceID: sourceId,
		InboxID:  agentBot.InboxID,
	})

	if err != nil {
		return -1, err
	}

	return response.ID, nil

}

func (agentBot *VoiceflowAgentBot) SendResponseBack(conversationId int, voiceflowResponse VoiceflowResponse) error {

	responseText := voiceflowResponse.Payload.Message

	responseText = strings.ReplaceAll(responseText, "\\n", "\n")

	if responseText == "" {
		utils.Logger.Info("The response from Rasa is empty. So no new message is created.")
		return nil
	}

	utils.LoggerSugar.Infof("ResponseText: %s", responseText)

	_, err := agentBot.ChatwootClient.CreateOutgoingMessage(conversationId, responseText)

	if err != nil {
		utils.LoggerSugar.Errorf("An error occurred while making the http request. %s", err)
		return err
	}

	return nil

}
