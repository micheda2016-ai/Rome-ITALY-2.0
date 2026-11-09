package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

const guildID = "1488262125708116198"

// ---------------------------------------------------------------------------
// STRUTTURE
// ---------------------------------------------------------------------------

type ServerConfig struct {
	PanelChannelID string
	EsitiChannelID string
	AuthRoleID     string
}

type GeneralTaxes struct {
	Conto            int
	Immatricolazione int
	Immobili         int
	IntervalDays     int
	CustomDeadline   string
}

type UserTaxOverride struct {
	Conto            int
	Immatricolazione int
	Immobili         int
}

type BankAccount struct {
	OwnerName   string
	RobloxName  string
	BankName    string
	Balance     int
	LastPayday  time.Time
	LastTaxPaid time.Time
}

type AnagrafeRecord struct {
	UserID      string
	RobloxUser  string
	NomeRP      string
	CognomeRP   string
	DataNascita string
	Sesso       string
}

type PortoDarmiRecord struct {
	UserID      string
	RobloxUser  string
	NomeRP      string
	Motivazione string
}

type ImmatricolazioneRecord struct {
	UserID      string
	RobloxUser  string
	Targa       string
	ModelloAuto string
}

type PatenteRecord struct {
	UserID     string
	RobloxUser string
	NomeRP     string
	Tipo       string
	Punti      int
}

// ---------------------------------------------------------------------------
// DATI IN MEMORIA
// ---------------------------------------------------------------------------

var mu sync.Mutex

var serverConfigs = make(map[string]*ServerConfig)
var userBankAccounts = make(map[string]BankAccount)
var dbAnagrafe = make(map[string]AnagrafeRecord)
var dbPortoDarmi = make(map[string]PortoDarmiRecord)
var dbImmatricolazioni = make(map[string]ImmatricolazioneRecord)
var dbPatenti = make(map[string]PatenteRecord)
var userTaxOverrides = make(map[string]UserTaxOverride)

var defaultTaxes = GeneralTaxes{
	Conto:            100,
	Immatricolazione: 200,
	Immobili:         500,
	IntervalDays:     7,
	CustomDeadline:   "Ogni 7 giorni",
}

var stipendioFisso = 13000

// ---------------------------------------------------------------------------
// HELPER
// ---------------------------------------------------------------------------

func getOrCreateConfig(key string) *ServerConfig {
	if serverConfigs[key] == nil {
		serverConfigs[key] = &ServerConfig{}
	}
	return serverConfigs[key]
}

func parseEuro(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func getUserTaxes(userID string) (UserTaxOverride, bool) {
	if o, ok := userTaxOverrides[userID]; ok {
		return o, true
	}
	return UserTaxOverride{
		Conto:            defaultTaxes.Conto,
		Immatricolazione: defaultTaxes.Immatricolazione,
		Immobili:         defaultTaxes.Immobili,
	}, false
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-3]) + "..."
}

func modalVal(d discordgo.ModalSubmitInteractionData, n int) string {
	if n >= len(d.Components) {
		return ""
	}
	row, ok := d.Components[n].(*discordgo.ActionsRow)
	if !ok || len(row.Components) == 0 {
		return ""
	}
	ti, ok := row.Components[0].(*discordgo.TextInput)
	if !ok {
		return ""
	}
	return ti.Value
}

func respond(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func inputRow(id, label string, style discordgo.TextInputStyle, value, placeholder string, required bool) discordgo.ActionsRow {
	return discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.TextInput{
			CustomID:    id,
			Label:       label,
			Style:       style,
			Value:       value,
			Placeholder: placeholder,
			Required:    required,
		},
	}}
}

func openModal(s *discordgo.Session, i *discordgo.InteractionCreate, customID, title string, rows ...discordgo.ActionsRow) {
	comps := make([]discordgo.MessageComponent, 0, len(rows))
	for _, r := range rows {
		comps = append(comps, r)
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID:   customID,
			Title:      title,
			Components: comps,
		},
	})
}

func esitiChannel(key, fallback string) string {
	cfg := getOrCreateConfig(key)
	if cfg.EsitiChannelID != "" {
		return cfg.EsitiChannelID
	}
	return fallback
}

func sendApproval(s *discordgo.Session, channelID string, embed *discordgo.MessageEmbed, userID, yesLabel, noLabel string) {
	btnYes := discordgo.Button{Label: yesLabel, Style: discordgo.SuccessButton, CustomID: "approve_yes_" + userID, Emoji: &discordgo.ComponentEmoji{Name: "✅"}}
	btnNo := discordgo.Button{Label: noLabel, Style: discordgo.DangerButton, CustomID: "approve_no_" + userID, Emoji: &discordgo.ComponentEmoji{Name: "❌"}}
	s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{btnYes, btnNo}},
		},
	})
}

func sendPanel(s *discordgo.Session, channelID string, embed *discordgo.MessageEmbed, btn *discordgo.Button) {
	msg := &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{embed}}
	if btn != nil {
		msg.Components = []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{*btn}},
		}
	}
	s.ChannelMessageSendComplex(channelID, msg)
}

// ---------------------------------------------------------------------------
// MAIN
// ---------------------------------------------------------------------------

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "10000"
	}
	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "Bot Rome Italy 2.0 Online!")
		})
		log.Printf("Server HTTP avviato sulla porta %s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Printf("Errore server HTTP: %v", err)
		}
	}()

	token := strings.TrimSpace(os.Getenv("TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("DISCORD_TOKEN"))
	}

	if token == "" {
		log.Println("⚠️ ERRORE: La variabile d'ambiente TOKEN non è impostata!")
	} else {
		token = strings.TrimPrefix(token, "Bot ")

		dg, err := discordgo.New("Bot " + token)
		if err != nil {
			log.Printf("Errore creazione sessione Discord: %v", err)
		} else {
			dg.AddHandler(interactionHandler)

			if err = dg.Open(); err != nil {
				log.Printf("Errore apertura connessione Discord: %v", err)
			} else {
				defer dg.Close()
				log.Println("✅ Bot 'Rome Italy 2.0' avviato con successo!")
				registerCommands(dg)
			}
		}
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

func registerCommands(s *discordgo.Session) {
	staffPerm := int64(discordgo.PermissionManageServer)

	setupSub := func(name, desc string) *discordgo.ApplicationCommandOption {
		return &discordgo.ApplicationCommandOption{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        name,
			Description: desc,
		}
	}

	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "conto",
			Description: "Visualizza il saldo e le informazioni del tuo conto corrente",
		},
		{
			Name:        "patente",
			Description: "Visualizza i punti e lo stato della tua patente di guida",
		},
		{
			Name:        "preleva",
			Description: "Preleva denaro contante dal tuo conto bancario",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "importo",
					Description: "Quantità di euro da prelevare",
					Required:    true,
					MinValue:    func() *float64 { v := float64(1); return &v }(),
				},
			},
		},
		{
			Name:        "deposita",
			Description: "Deposita denaro contante sul tuo conto bancario",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "importo",
					Description: "Quantità di euro da depositare",
					Required:    true,
					MinValue:    func() *float64 { v := float64(1); return &v }(),
				},
			},
		},
		{
			Name:                     "modifica",
			Description:              "Comandi di modifica dati anagrafici e bancari per lo Staff",
			DefaultMemberPermissions: &staffPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "conto",
					Description: "Modifica il saldo del conto bancario di un utente",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionUser, Name: "utente", Description: "Utente Discord", Required: true},
						{Type: discordgo.ApplicationCommandOptionInteger, Name: "nuovo_saldo", Description: "Nuovo saldo in euro", Required: true},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "cittadinanza",
					Description: "Modifica i dati anagrafici di un cittadino",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionUser, Name: "utente", Description: "Utente Discord", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "nome_cognome", Description: "Nuovo Nome e Cognome RP", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "roblox", Description: "Nuovo Username Roblox", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "data_nascita", Description: "Data di Nascita", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "sesso", Description: "Sesso (M/F)", Required: true},
					},
				},
			},
		},
		{
			Name:                     "decurta",
			Description:              "Decurta punti dalla patente di un cittadino",
			DefaultMemberPermissions: &staffPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "punti-patente",
					Description: "Rimuovi punti patente a un utente",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionUser, Name: "utente", Description: "Cittadino sanzionato", Required: true},
						{Type: discordgo.ApplicationCommandOptionInteger, Name: "punti", Description: "Numero di punti da decurtare", Required: true, MinValue: func() *float64 { v := float64(1); return &v }()},
						{Type: discordgo.ApplicationCommandOptionString, Name: "motivo", Description: "Motivazione dell'infrazione", Required: true},
					},
				},
			},
		},
		{
			Name:                     "ban",
			Description:              "Emetti un provvedimento di ban per un utente",
			DefaultMemberPermissions: &staffPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "utente", Description: "Utente Discord", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "roblox", Description: "Username Roblox", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "motivo", Description: "Motivo del ban", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "durata", Description: "Durata", Required: true},
			},
		},
		{
			Name:                     "warn",
			Description:              "Emetti un avvertimento a un utente",
			DefaultMemberPermissions: &staffPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "utente", Description: "Utente Discord", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "roblox", Description: "Username Roblox", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "motivo", Description: "Motivo", Required: true},
				{Type: discordgo.ApplicationCommandOptionInteger, Name: "numero", Description: "Numero warn (1-3)", Required: true, MinValue: func() *float64 { v := float64(1); return &v }(), MaxValue: 3},
			},
		},
		{
			Name:                     "visualizza",
			Description:              "Visualizza database Staff",
			DefaultMemberPermissions: &staffPerm,
			Options: []*discordgo.ApplicationCommandOption{
				setupSub("anagrafe", "Anagrafe registrata"),
				setupSub("porto-darmi", "Porto d'armi"),
				setupSub("immatricolazioni", "Veicoli immatricolati"),
				setupSub("patenti", "Patenti attive"),
			},
		},
		{
			Name:                     "setup",
			Description:              "Configura pannelli e canali",
			DefaultMemberPermissions: &staffPerm,
			Options: []*discordgo.ApplicationCommandOption{
				setupSub("server", "Configura canali e ruoli globali"),
				setupSub("apri-conto", "Pannello conto bancario"),
				setupSub("busta-paga", "Pannello busta paga"),
				setupSub("cittadinanza", "Pannello cittadinanza"),
				setupSub("patente", "Pannello richiesta patente di guida"),
				setupSub("porto-darmi", "Pannello porto d'armi"),
				setupSub("immatricolazione", "Pannello PRA"),
				setupSub("registra-arresto", "Pannello arresti FDO"),
				setupSub("emetti-multa", "Pannello multe"),
				setupSub("tasse", "Pannello tasse"),
				setupSub("staff-panel", "Pannello staff"),
			},
		},
	}

	s.ApplicationCommandBulkOverwrite(s.State.User.ID, "", []*discordgo.ApplicationCommand{})
	s.ApplicationCommandBulkOverwrite(s.State.User.ID, guildID, commands)
}

// ---------------------------------------------------------------------------
// ROUTING
// ---------------------------------------------------------------------------

func interactionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	mu.Lock()
	defer mu.Unlock()

	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		handleSlashCommands(s, i)
	case discordgo.InteractionMessageComponent:
		handleComponents(s, i)
	case discordgo.InteractionModalSubmit:
		handleModalSubmit(s, i)
	}
}

// ---------------------------------------------------------------------------
// SLASH COMMANDS
// ---------------------------------------------------------------------------

func handleSlashCommands(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()

	if i.Member == nil {
		respond(s, i, "❌ Questo comando funziona solo dentro il server.")
		return
	}

	userID := i.Member.User.ID

	if data.Name == "conto" {
		account, exists := userBankAccounts[userID]
		if !exists {
			respond(s, i, "❌ Non possiedi un conto corrente registrato!")
			return
		}

		t, personalized := getUserTaxes(userID)
		tipo := "Generali"
		if personalized {
			tipo = "Personalizzate"
		}

		embed := &discordgo.MessageEmbed{
			Title: "🏛️ ESTRATTO CONTO BANCARIO",
			Color: 0x2ECC71,
			Description: fmt.Sprintf(
				"**Utente Discord:** <@%s>\n**Intestatario RP:** %s\n**Utente Roblox:** `@%s`\n**Banca:** %s\n💰 **Saldo:** `€ %d`\n\n"+
					"🧾 **Tasse (%s):**\n• Conto: € %d\n• Immatricolazione: € %d\n• Immobili: € %d\n**Totale:** € %d",
				userID, account.OwnerName, account.RobloxName, strings.ToUpper(account.BankName), account.Balance,
				tipo, t.Conto, t.Immatricolazione, t.Immobili, t.Conto+t.Immatricolazione+t.Immobili),
		}
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	if data.Name == "patente" {
		pat, exists := dbPatenti[userID]
		if !exists {
			respond(s, i, "❌ Non possiedi una patente di guida registrata!")
			return
		}
		embed := &discordgo.MessageEmbed{
			Title: "🪪 TESSERINO PATENTE DI GUIDA",
			Color: 0xF1C40F,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Titolare", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
				{Name: "Intestatario RP", Value: pat.NomeRP, Inline: true},
				{Name: "Tipo Patente", Value: pat.Tipo, Inline: true},
				{Name: "Roblox", Value: fmt.Sprintf("`@%s`", pat.RobloxUser), Inline: true},
				{Name: "Punti Rimanenti", Value: fmt.Sprintf("`%d / 30 punti`", pat.Punti), Inline: false},
			},
		}
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	if data.Name == "preleva" || data.Name == "deposita" {
		importo := int(data.Options[0].IntValue())
		account, exists := userBankAccounts[userID]
		if !exists {
			respond(s, i, "❌ Non possiedi un conto corrente registrato.")
			return
		}

		if data.Name == "preleva" {
			if account.Balance < importo {
				respond(s, i, fmt.Sprintf("⚠️ Saldo insufficiente! Saldo attuale: `€ %d`", account.Balance))
				return
			}
			account.Balance -= importo
			userBankAccounts[userID] = account
			respond(s, i, fmt.Sprintf("💵 Prelevati **€ %d**. Nuovo saldo: `€ %d`", importo, account.Balance))
		} else {
			account.Balance += importo
			userBankAccounts[userID] = account
			respond(s, i, fmt.Sprintf("💳 Depositati **€ %d**. Nuovo saldo: `€ %d`", importo, account.Balance))
		}
		return
	}

	if data.Name == "ban" {
		var targetUser *discordgo.User
		var roblox, motivo, durata string
		for _, opt := range data.Options {
			switch opt.Name {
			case "utente":
				targetUser = opt.UserValue(s)
			case "roblox":
				roblox = opt.StringValue()
			case "motivo":
				motivo = opt.StringValue()
			case "durata":
				durata = opt.StringValue()
			}
		}
		if targetUser == nil {
			respond(s, i, "❌ Utente non valido.")
			return
		}

		embed := &discordgo.MessageEmbed{
			Title: "🔨 PROVVEDIMENTO DI BAN",
			Color: 0xE74C3C,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Utente", Value: fmt.Sprintf("<@%s>", targetUser.ID), Inline: true},
				{Name: "Roblox", Value: fmt.Sprintf("`@%s`", roblox), Inline: true},
				{Name: "Durata", Value: durata, Inline: true},
				{Name: "Motivo", Value: motivo, Inline: false},
			},
		}
		respond(s, i, "🔨 Ban emesso con successo!")
		s.ChannelMessageSendComplex(esitiChannel("ban", i.ChannelID), &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{embed}})
		if ch, err := s.UserChannelCreate(targetUser.ID); err == nil {
			s.ChannelMessageSend(ch.ID, fmt.Sprintf("⚠️ Sei stato **BANNATO**. Motivo: %s | Durata: %s", motivo, durata))
		}
		return
	}

	if data.Name == "warn" {
		var targetUser *discordgo.User
		var roblox, motivo string
		var numero int
		for _, opt := range data.Options {
			switch opt.Name {
			case "utente":
				targetUser = opt.UserValue(s)
			case "roblox":
				roblox = opt.StringValue()
			case "motivo":
				motivo = opt.StringValue()
			case "numero":
				numero = int(opt.IntValue())
			}
		}
		if targetUser == nil {
			respond(s, i, "❌ Utente non valido.")
			return
		}

		embed := &discordgo.MessageEmbed{
			Title: fmt.Sprintf("⚠️ AVVERTIMENTO (WARN %d/3)", numero),
			Color: 0xF1C40F,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Utente", Value: fmt.Sprintf("<@%s>", targetUser.ID), Inline: true},
				{Name: "Roblox", Value: fmt.Sprintf("`@%s`", roblox), Inline: true},
				{Name: "Livello", Value: fmt.Sprintf("%d/3", numero), Inline: true},
				{Name: "Motivo", Value: motivo, Inline: false},
			},
		}
		respond(s, i, "⚠️ Warn emesso.")
		s.ChannelMessageSendComplex(esitiChannel("warn", i.ChannelID), &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{embed}})
		if ch, err := s.UserChannelCreate(targetUser.ID); err == nil {
			s.ChannelMessageSend(ch.ID, fmt.Sprintf("⚠️ Hai ricevuto un **Warn (%d/3)**. Motivo: %s", numero, motivo))
		}
		return
	}

	if len(data.Options) == 0 {
		return
	}
	sub := data.Options[0].Name

	if data.Name == "visualizza" {
		var sb strings.Builder
		switch sub {
		case "anagrafe":
			sb.WriteString("📋 **ANAGRAFE:**\n\n")
			for _, r := range dbAnagrafe {
				sb.WriteString(fmt.Sprintf("• <@%s> | %s %s | `@%s`\n", r.UserID, r.NomeRP, r.CognomeRP, r.RobloxUser))
			}
		case "porto-darmi":
			sb.WriteString("🔫 **PORTO D'ARMI:**\n\n")
			for _, r := range dbPortoDarmi {
				sb.WriteString(fmt.Sprintf("• <@%s> | %s | `@%s`\n", r.UserID, r.NomeRP, r.RobloxUser))
			}
		case "immatricolazioni":
			sb.WriteString("🚗 **VEICOLI:**\n\n")
			for _, r := range dbImmatricolazioni {
				sb.WriteString(fmt.Sprintf("• <@%s> | `%s` (%s)\n", r.UserID, r.Targa, r.ModelloAuto))
			}
		case "patenti":
			sb.WriteString("🪪 **PATENTI:**\n\n")
			for _, r := range dbPatenti {
				sb.WriteString(fmt.Sprintf("• <@%s> | %s | Tipo: **%s** | Punti: `%d/30`\n", r.UserID, r.NomeRP, r.Tipo, r.Punti))
			}
		}
		respond(s, i, truncate(sb.String(), 1900))
		return
	}

	if data.Name == "modifica" {
		targetUser := data.Options[0].Options[0].UserValue(s)
		targetID := targetUser.ID

		if sub == "conto" {
			nuovoSaldo := int(data.Options[0].Options[1].IntValue())
			acc := userBankAccounts[targetID]
			acc.Balance = nuevoSaldo
			userBankAccounts[targetID] = acc
			respond(s, i, fmt.Sprintf("✅ Saldo conto di <@%s> modificato a € %d.", targetID, nuovoSaldo))
			return
		}

		if sub == "cittadinanza" {
			nc := data.Options[0].Options[1].StringValue()
			rob := data.Options[0].Options[2].StringValue()
			dat := data.Options[0].Options[3].StringValue()
			ses := data.Options[0].Options[4].StringValue()
			parts := strings.SplitN(nc, " ", 2)
			n, c := nc, ""
			if len(parts) == 2 {
				n, c = parts[0], parts[1]
			}
			dbAnagrafe[targetID] = AnagrafeRecord{UserID: targetID, RobloxUser: rob, NomeRP: n, CognomeRP: c, DataNascita: dat, Sesso: ses}
			respond(s, i, fmt.Sprintf("✅ Anagrafica di <@%s> aggiornata.", targetID))
			return
		}
	}

	if data.Name == "decurta" && sub == "punti-patente" {
		targetUser := data.Options[0].Options[0].UserValue(s)
		punti := int(data.Options[0].Options[1].IntValue())
		motivo := data.Options[0].Options[2].StringValue()
		targetID := targetUser.ID

		pat, exists := dbPatenti[targetID]
		if !exists {
			respond(s, i, "❌ L'utente non ha una patente registrata!")
			return
		}

		pat.Punti -= punti
		if pat.Punti < 0 {
			pat.Punti = 0
		}
		dbPatenti[targetID] = pat

		respond(s, i, fmt.Sprintf("✅ Decurtati **%d punti** a <@%s>. Rimasti: **%d / 30**", punti, targetID, pat.Punti))

		embedEsito := &discordgo.MessageEmbed{
			Title: "🚨 SANZIONE / DECURTAZIONE PATENTE",
			Color: 0xE74C3C,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Agente", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
				{Name: "Punti Tolti", Value: fmt.Sprintf("-%d", punti), Inline: true},
				{Name: "Punti Residui", Value: fmt.Sprintf("%d / 30", pat.Punti), Inline: true},
				{Name: "Motivo", Value: motivo, Inline: false},
			},
		}
		s.ChannelMessageSendComplex(esitiChannel("patente", i.ChannelID), &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{embedEsito}})
		if ch, err := s.UserChannelCreate(targetID); err == nil {
			s.ChannelMessageSendEmbed(ch.ID, embedEsito)
		}
		return
	}

	if data.Name == "setup" {
		switch sub {
		case "server":
			sendServerSetupPanel(s, i)
		case "tasse":
			sendTasseSetupPanel(s, i)
		default:
			sendPanelPreview(s, i, sub)
		}
	}
}

// ---------------------------------------------------------------------------
// SETUP PANELS
// ---------------------------------------------------------------------------

func channelInfoText(key string) string {
	cfg := getOrCreateConfig(key)
	if cfg.PanelChannelID == "" {
		return "⚠️ *Canale non impostato.*"
	}
	return fmt.Sprintf("📍 *Canale:* <#%s>", cfg.PanelChannelID)
}

func sendTasseSetupPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title: "🏛️ GESTIONE FISCALE E TASSE",
		Description: fmt.Sprintf("Conto: € %d | Immatricolazione: € %d | Immobili: € %d\nScadenza: %s\n\n%s",
			defaultTaxes.Conto, defaultTaxes.Immatricolazione, defaultTaxes.Immobili, defaultTaxes.CustomDeadline, channelInfoText("tasse")),
		Color: 0x95A5A6,
	}
	row := discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{Label: "Invia Pannello", Style: discordgo.SuccessButton, CustomID: "send_panel_to_channel_tasse"},
	}}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: []discordgo.MessageComponent{row}, Flags: discordgo.MessageFlagsEphemeral},
	})
}

func sendPanelPreview(s *discordgo.Session, i *discordgo.InteractionCreate, serviceKey string) {
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("📤 INVIA PANNELLO: %s", strings.ToUpper(serviceKey)),
		Description: channelInfoText(serviceKey),
		Color:       0x3498DB,
	}
	btn := discordgo.Button{Label: "Invia nel Canale", Style: discordgo.SuccessButton, CustomID: "send_panel_to_channel_" + serviceKey}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{btn}}}, Flags: discordgo.MessageFlagsEphemeral},
	})
}

func sendServerSetupPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{Title: "⚙️ CONFIGURAZIONE GLOBALE", Description: "Seleziona il comando:", Color: 0x0099FF}
	sm := discordgo.SelectMenu{
		CustomID:    "setup_server_select",
		Placeholder: "Scegli...",
		Options: []discordgo.SelectMenuOption{
			{Label: "Cittadinanza", Value: "cittadinanza", Emoji: &discordgo.ComponentEmoji{Name: "🇮🇹"}},
			{Label: "Patente di Guida", Value: "patente", Emoji: &discordgo.ComponentEmoji{Name: "🪪"}},
			{Label: "Porto d'Armi", Value: "porto-darmi", Emoji: &discordgo.ComponentEmoji{Name: "🔫"}},
			{Label: "Apri Conto", Value: "apri-conto", Emoji: &discordgo.ComponentEmoji{Name: "💳"}},
			{Label: "Busta Paga", Value: "busta-paga", Emoji: &discordgo.ComponentEmoji{Name: "💵"}},
			{Label: "Immatricolazione", Value: "immatricolazione", Emoji: &discordgo.ComponentEmoji{Name: "🚗"}},
			{Label: "Arresti FDO", Value: "registra-arresto", Emoji: &discordgo.ComponentEmoji{Name: "🚨"}},
			{Label: "Multe", Value: "emetti-multa", Emoji: &discordgo.ComponentEmoji{Name: "📄"}},
			{Label: "Tasse", Value: "tasse", Emoji: &discordgo.ComponentEmoji{Name: "🏛️"}},
		},
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{sm}}}, Flags: discordgo.MessageFlagsEphemeral},
	})
}

// ---------------------------------------------------------------------------
// COMPONENTI
// ---------------------------------------------------------------------------

func handleComponents(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	values := i.MessageComponentData().Values

	if i.Member == nil {
		return
	}

	if strings.HasPrefix(customID, "send_panel_to_channel_") {
		sk := strings.TrimPrefix(customID, "send_panel_to_channel_")
		tc := getOrCreateConfig(sk).PanelChannelID
		if tc == "" {
			tc = i.ChannelID
		}
		publishPublicPanel(s, tc, sk)
		respond(s, i, fmt.Sprintf("✅ Pannello inviato in <#%s>!", tc))
		return
	}

	if strings.HasPrefix(customID, "approve_yes_") || strings.HasPrefix(customID, "approve_no_") {
		parts := strings.Split(customID, "_")
		if len(parts) < 3 || len(i.Message.Embeds) == 0 {
			return
		}
		azione := parts[1]
		targetUserID := parts[2]

		embedOrig := i.Message.Embeds[0]
		newEmbed := *embedOrig
		if azione == "yes" {
			newEmbed.Color = 0x2ECC71
			newEmbed.Title = "✅ APPROVATO DALLO STAFF"

			if strings.Contains(newEmbed.Title, "PATENTE") || strings.Contains(newEmbed.Description, "Patente") {
				tipoPatente := "Tipo B (Veicoli)"
				for _, f := range newEmbed.Fields {
					if f.Name == "Tipo Richiesto" {
						tipoPatente = f.Value
					}
				}
				dbPatenti[targetUserID] = PatenteRecord{UserID: targetUserID, RobloxUser: "Roblox", NomeRP: "Cittadino", Tipo: tipoPatente, Punti: 30}
			}
		} else {
			newEmbed.Color = 0xE74C3C
			newEmbed.Title = "❌ RIFIUTATO DALLO STAFF"
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{&newEmbed}, Components: []discordgo.MessageComponent{}},
		})

		if ch, err := s.UserChannelCreate(targetUserID); err == nil {
			if azione == "yes" {
				s.ChannelMessageSend(ch.ID, "🎉 La tua richiesta è stata **APPROVATA**! (Se era per la patente, parti con 30 punti).")
			} else {
				s.ChannelMessageSend(ch.ID, "❌ La tua richiesta è stata **RIFIUTATA**.")
			}
		}
		return
	}

	if customID == "setup_server_select" {
		selected := "cittadinanza"
		if len(values) > 0 {
			selected = values[0]
		}
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("🛠️ Configura: **%s**", strings.ToUpper(selected)),
				Flags:   discordgo.MessageFlagsEphemeral,
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{CustomID: "selchanpanel_" + selected, Placeholder: "Canale pannello (#)...", MenuType: discordgo.ChannelSelectMenu},
					}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{CustomID: "selchanesiti_" + selected, Placeholder: "Canale esiti (#)...", MenuType: discordgo.ChannelSelectMenu},
					}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{CustomID: "selroleauth_" + selected, Placeholder: "Ruolo autorizzato (@)...", MenuType: discordgo.RoleSelectMenu},
					}},
				},
			},
		})
		return
	}

	if strings.HasPrefix(customID, "selchanpanel_") || strings.HasPrefix(customID, "selchanesiti_") || strings.HasPrefix(customID, "selroleauth_") {
		if len(values) > 0 {
			v := values[0]
			switch {
			case strings.HasPrefix(customID, "selchanpanel_"):
				getOrCreateConfig(strings.TrimPrefix(customID, "selchanpanel_")).PanelChannelID = v
			case strings.HasPrefix(customID, "selchanesiti_"):
				getOrCreateConfig(strings.TrimPrefix(customID, "selchanesiti_")).EsitiChannelID = v
			case strings.HasPrefix(customID, "selroleauth_"):
				getOrCreateConfig(strings.TrimPrefix(customID, "selroleauth_")).AuthRoleID = v
			}
		}
		respond(s, i, "✅ Salvato!")
		return
	}

	switch customID {
	case "btn_apri_conto":
		selectBanche := discordgo.SelectMenu{
			CustomID:    "select_banca_choice",
			Placeholder: "Seleziona banca...",
			Options: []discordgo.SelectMenuOption{
				{Label: "Banca Sella", Value: "banca_sella"},
				{Label: "Poste Italiane", Value: "poste"},
				{Label: "Intesa Sanpaolo", Value: "intesa"},
			},
		}
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Scegli banca (Bonus € 7.000):", Flags: discordgo.MessageFlagsEphemeral, Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{selectBanche}}}},
		})

	case "select_banca_choice":
		selected := "banca"
		if len(values) > 0 {
			selected = values[0]
		}
		openModal(s, i, "modal_conferma_conto_"+selected, "Apertura Conto",
			inputRow("n_rp", "Nome e Cognome RP", discordgo.TextInputShort, "", "Mario Rossi", true),
			inputRow("r_user", "Username Roblox", discordgo.TextInputShort, "", "RobloxUser", true),
		)

	case "btn_richiedi_patente":
		openModal(s, i, "modal_patente", "Richiesta Patente di Guida",
			inputRow("p_nome", "Nome e Cognome RP", discordgo.TextInputShort, "", "Mario Rossi", true),
			inputRow("p_roblox", "Username Roblox", discordgo.TextInputShort, "", "RobloxUser", true),
			inputRow("p_tipo", "Tipo (A: Moto, B: Veicoli, C: Pulmini, D: Camion)", discordgo.TextInputShort, "", "Tipo B", true),
		)

	case "btn_richiedi_cittadinanza":
		openModal(s, i, "modal_cittadinanza", "Cittadinanza",
			inputRow("c_nc", "Nome e Cognome RP", discordgo.TextInputShort, "", "Mario Rossi", true),
			inputRow("c_rob", "Username Roblox", discordgo.TextInputShort, "", "RobloxUser", true),
			inputRow("c_dat", "Data di Nascita", discordgo.TextInputShort, "", "01/01/2000", true),
			inputRow("c_ses", "Sesso (M/F)", discordgo.TextInputShort, "", "M", true),
		)

	case "btn_richiedi_porto_darmi":
		openModal(s, i, "modal_porto_darmi", "Porto d'Armi",
			inputRow("pd_nome", "Nome e Cognome RP", discordgo.TextInputShort, "", "", true),
			inputRow("pd_rob", "Username Roblox", discordgo.TextInputShort, "", "", true),
			inputRow("pd_mot", "Motivazione", discordgo.TextInputParagraph, "", "", true),
		)

	case "btn_immatricolazione":
		openModal(s, i, "modal_immatricolazione", "Immatricolazione Veicolo",
			inputRow("im_rob", "Username Roblox", discordgo.TextInputShort, "", "", true),
			inputRow("im_mod", "Modello Auto", discordgo.TextInputShort, "", "", true),
			inputRow("im_tar", "Targa", discordgo.TextInputShort, "", "", true),
		)

	case "btn_registra_arresto":
		openModal(s, i, "modal_registra_arresto", "Arresto FDO",
			inputRow("ar_arr", "Utente Arrestato (Roblox)", discordgo.TextInputShort, "", "", true),
			inputRow("ar_rea", "Reati", discordgo.TextInputParagraph, "", "", true),
			inputRow("ar_pri", "Prigione", discordgo.TextInputShort, "", "", true),
			inputRow("ar_cau", "Cauzione", discordgo.TextInputShort, "", "5000", false),
		)

	case "btn_emetti_multa":
		openModal(s, i, "modal_emetti_multa", "Multa",
			inputRow("mu_mul", "Utente Multato (Roblox)", discordgo.TextInputShort, "", "", true),
			inputRow("mu_imp", "Importo (€)", discordgo.TextInputShort, "", "150", true),
			inputRow("mu_mot", "Motivo", discordgo.TextInputParagraph, "", "", true),
			inputRow("mu_tar", "Targa", discordgo.TextInputShort, "", "", false),
		)

	case "btn_paga_tasse":
		userID := i.Member.User.ID
		acc, ok := userBankAccounts[userID]
		if !ok {
			respond(s, i, "❌ Apri prima un conto corrente!")
			return
		}
		t, _ := getUserTaxes(userID)
		tot := t.Conto + t.Immatricolazione + t.Immobili
		if acc.Balance < tot {
			respond(s, i, fmt.Sprintf("⚠️ Saldo insufficiente! Tasse: € %d, Saldo: € %d", tot, acc.Balance))
			return
		}
		acc.Balance -= tot
		acc.LastTaxPaid = time.Now()
		userBankAccounts[userID] = acc
		respond(s, i, fmt.Sprintf("🏛️ Tasse pagate con successo! Detratti € %d. Saldo: € %d", tot, acc.Balance))

	case "btn_riscuoti_stipendio":
		userID := i.Member.User.ID
		acc, ok := userBankAccounts[userID]
		if !ok {
			respond(s, i, "❌ Apri prima un conto corrente!")
			return
		}
		acc.Balance += stipendioFisso
		acc.LastPayday = time.Now()
		userBankAccounts[userID] = acc
		respond(s, i, fmt.Sprintf("✅ Busta paga riscossa! Accreditati € %d. Saldo: € %d", stipendioFisso, acc.Balance))
	}
}

// ---------------------------------------------------------------------------
// PUBBLICAZIONE PANNELLI
// ---------------------------------------------------------------------------

func publishPublicPanel(s *discordgo.Session, channelID string, serviceKey string) {
	switch serviceKey {
	case "apri-conto":
		sendPanel(s, channelID, &discordgo.MessageEmbed{Title: "💳 APERTURA CONTO", Description: "Ricevi € 7.000 di bonus.", Color: 0x2ECC71}, &discordgo.Button{Label: "Apri Conto", Style: discordgo.SuccessButton, CustomID: "btn_apri_conto"})
	case "patente":
		sendPanel(s, channelID, &discordgo.MessageEmbed{Title: "🪪 RICHIESTA PATENTE DI GUIDA", Description: "Clicca **Compila** per richiedere la patente.\n• **Tipo A:** Moto\n• **Tipo B:** Veicoli\n• **Tipo C:** Pulmini\n• **Tipo D:** Camion\nParti con **30 punti**.", Color: 0xF1C40F}, &discordgo.Button{Label: "Compila", Style: discordgo.PrimaryButton, CustomID: "btn_richiedi_patente"})
	case "porto-darmi":
		sendPanel(s, channelID, &discordgo.MessageEmbed{Title: "🔫 PORTO D'ARMI", Description: "Richiedi porto d'armi.", Color: 0xE67E22}, &discordgo.Button{Label: "Richiedi", Style: discordgo.PrimaryButton, CustomID: "btn_richiedi_porto_darmi"})
	case "busta-paga":
		sendPanel(s, channelID, &discordgo.MessageEmbed{Title: "💵 BUSTA PAGA", Description: "Riscuoti stipendio.", Color: 0xF1C40F}, &discordgo.Button{Label: "Riscuoti", Style: discordgo.PrimaryButton, CustomID: "btn_riscuoti_stipendio"})
	case "cittadinanza":
		sendPanel(s, channelID, &discordgo.MessageEmbed{Title: "🇮🇹 CITTADINANZA", Description: "Richiedi documenti.", Color: 0x3498DB}, &discordgo.Button{Label: "Richiedi", Style: discordgo.PrimaryButton, CustomID: "btn_richiedi_cittadinanza"})
	case "immatricolazione":
		sendPanel(s, channelID, &discordgo.MessageEmbed{Title: "🚗 IMMATRICOLAZIONE", Description: "Registra veicolo.", Color: 0x9B59B6}, &discordgo.Button{Label: "Immatricola", Style: discordgo.PrimaryButton, CustomID: "btn_immatricolazione"})
	case "registra-arresto":
		sendPanel(s, channelID, &discordgo.MessageEmbed{Title: "🚨 ARRESTI FDO", Description: "Registra arresto.", Color: 0xE74C3C}, &discordgo.Button{Label: "Registra", Style: discordgo.DangerButton, CustomID: "btn_registra_arresto"})
	case "emetti-multa":
		sendPanel(s, channelID, &discordgo.MessageEmbed{Title: "📄 MULTA", Description: "Emetti sanzione.", Color: 0xE74C3C}, &discordgo.Button{Label: "Emetti", Style: discordgo.DangerButton, CustomID: "btn_emetti_multa"})
	case "tasse":
		sendPanel(s, channelID, &discordgo.MessageEmbed{Title: "🏛️ TASSE", Description: "Paga le imposte.", Color: 0x95A5A6}, &discordgo.Button{Label: "Paga Tasse", Style: discordgo.SuccessButton, CustomID: "btn_paga_tasse"})
	}
}

// ---------------------------------------------------------------------------
// MODAL SUBMIT
// ---------------------------------------------------------------------------

func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	customID := data.CustomID
	if i.Member == nil {
		return
	}
	userID := i.Member.User.ID

	if strings.HasPrefix(customID, "modal_conferma_conto_") {
		banca := strings.TrimPrefix(customID, "modal_conferma_conto_")
		rUsr := modalVal(data, 1)

		userBankAccounts[userID] = BankAccount{OwnerName: modalVal(data, 0), RobloxName: rUsr, BankName: banca, Balance: 7000}
		respond(s, i, "🎉 Conto aperto! Accreditati € 7.000 di bonus.")

		embed := &discordgo.MessageEmbed{
			Title: "💳 APERTURA CONTO",
			Color: 0x2ECC71,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Utente", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
				{Name: "Roblox", Value: rUsr, Inline: true},
				{Name: "Nome RP", Value: modalVal(data, 0), Inline: true},
				{Name: "Banca", Value: strings.ToUpper(banca), Inline: true},
			},
		}
		sendApproval(s, esitiChannel("apri-conto", i.ChannelID), embed, userID, "Accetta", "Rifiuta")
		return
	}

	switch customID {
	case "modal_patente":
		rUsr := modalVal(data, 1)
		tPat := modalVal(data, 2)

		respond(s, i, "🪪 Richiesta patente inviata allo staff!")

		embed := &discordgo.MessageEmbed{
			Title: "🪪 RICHIESTA PATENTE DI GUIDA",
			Color: 0xF1C40F,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Richiedente", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
				{Name: "Roblox", Value: fmt.Sprintf("`@%s`", rUsr), Inline: true},
				{Name: "Nome RP", Value: modalVal(data, 0), Inline: true},
				{Name: "Tipo Richiesto", Value: tPat, Inline: true},
				{Name: "Punti Iniziali", Value: "30 Punti", Inline: true},
			},
		}
		sendApproval(s, esitiChannel("patente", i.ChannelID), embed, userID, "Accetta", "Rifiuta")

	case "modal_porto_darmi":
		rUsr := modalVal(data, 1)
		mot := modalVal(data, 2)

		respond(s, i, "📜 Richiesta porto d'armi inviata.")
		embed := &discordgo.MessageEmbed{
			Title: "🔫 PORTO D'ARMI",
			Color: 0xE67E22,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Utente", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
				{Name: "Roblox", Value: rUsr, Inline: true},
				{Name: "Motivazione", Value: mot, Inline: false},
			},
		}
		sendApproval(s, esitiChannel("porto-darmi", i.ChannelID), embed, userID, "Accetta", "Rifiuta")

	case "modal_cittadinanza":
		nc := modalVal(data, 0)
		rob := modalVal(data, 1)
		dat := modalVal(data, 2)
		ses := modalVal(data, 3)

		parts := strings.SplitN(nc, " ", 2)
		n, c := nc, ""
		if len(parts) == 2 {
			n, c = parts[0], parts[1]
		}
		dbAnagrafe[userID] = AnagrafeRecord{UserID: userID, RobloxUser: rob, NomeRP: n, CognomeRP: c, DataNascita: dat, Sesso: ses}

		respond(s, i, "🇮🇹 Cittadinanza inviata.")
		embed := &discordgo.MessageEmbed{
			Title: "🇮🇹 CITTADINANZA",
			Color: 0x3498DB,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Utente", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
				{Name: "Nome e Cognome", Value: nc, Inline: true},
				{Name: "Roblox", Value: rob, Inline: true},
			},
		}
		sendApproval(s, esitiChannel("cittadinanza", i.ChannelID), embed, userID, "Accetta", "Rifiuta")

	case "modal_immatricolazione":
		rob := modalVal(data, 0)
		mod := modalVal(data, 1)
		tar := strings.ToUpper(strings.TrimSpace(modalVal(data, 2)))

		dbImmatricolazioni[tar] = ImmatricolazioneRecord{UserID: userID, RobloxUser: rob, Targa: tar, ModelloAuto: mod}
		respond(s, i, "🚗 Immatricolazione inviata.")
		embed := &discordgo.MessageEmbed{
			Title: "🚗 IMMATRICOLAZIONE",
			Color: 0x9B59B6,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Utente", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
				{Name: "Modello", Value: mod, Inline: true},
				{Name: "Targa", Value: tar, Inline: true},
			},
		}
		sendApproval(s, esitiChannel("immatricolazione", i.ChannelID), embed, userID, "Accetta", "Rifiuta")

	case "modal_registra_arresto":
		arr := modalVal(data, 0)
		rea := modalVal(data, 1)

		respond(s, i, "🚨 Verbale registrato.")
		embed := &discordgo.MessageEmbed{
			Title: "🚨 ARRESTO",
			Color: 0xE74C3C,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Agente", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
				{Name: "Arrestato", Value: arr, Inline: true},
				{Name: "Reati", Value: rea, Inline: false},
			},
		}
		sendApproval(s, esitiChannel("registra-arresto", i.ChannelID), embed, userID, "Approva", "Annulla")

	case "modal_emetti_multa":
		mul := modalVal(data, 0)
		imp := modalVal(data, 1)
		mot := modalVal(data, 2)

		respond(s, i, "📄 Multa emessa.")
		embed := &discordgo.MessageEmbed{
			Title: "📄 MULTA",
			Color: 0xE74C3C,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Agente", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
				{Name: "Multato", Value: mul, Inline: true},
				{Name: "Importo", Value: "€ " + imp, Inline: true},
				{Name: "Motivo", Value: mot, Inline: false},
			},
		}
		sendApproval(s, esitiChannel("emetti-multa", i.ChannelID), embed, userID, "Approva", "Annulla")
	}
}
