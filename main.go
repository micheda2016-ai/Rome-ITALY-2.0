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

// Tasse generali (valgono per tutti, se non hanno tasse personalizzate)
type GeneralTaxes struct {
	Conto            int
	Immatricolazione int
	Immobili         int
	IntervalDays     int
	CustomDeadline   string
}

// Tasse personalizzate per singolo utente
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

// ---------------------------------------------------------------------------
// DATI IN MEMORIA
// ---------------------------------------------------------------------------

var mu sync.Mutex // protegge tutte le mappe qui sotto

var serverConfigs = make(map[string]*ServerConfig)
var userBankAccounts = make(map[string]BankAccount)
var dbAnagrafe = make(map[string]AnagrafeRecord)
var dbPortoDarmi = make(map[string]PortoDarmiRecord)
var dbImmatricolazioni = make(map[string]ImmatricolazioneRecord)
var userTaxOverrides = make(map[string]UserTaxOverride)

var defaultTaxes = GeneralTaxes{
	Conto:            100,
	Immatricolazione: 200,
	Immobili:         500,
	IntervalDays:     7,
	CustomDeadline:   "Ogni 7 giorni",
}

// Stipendio settimanale uguale per tutti
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

// Restituisce le tasse dell'utente; true se sono personalizzate
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

	token := strings.TrimSpace(os.Getenv("DISCORD_TOKEN"))
	if token == "" {
		log.Println("⚠️ ERRORE: La variabile d'ambiente DISCORD_TOKEN non è impostata!")
	} else {
		token = strings.TrimPrefix(token, "Bot ")

		dg, err := discordgo.New("Bot " + token)
		if err != nil {
			log.Printf("Errore durante la creazione della sessione Discord: %v", err)
		} else {
			dg.AddHandler(interactionHandler)

			if err = dg.Open(); err != nil {
				log.Printf("Errore durante l'apertura della connessione a Discord: %v", err)
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

	log.Println("Arresto del bot...")
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
			Description: "Visualizza il saldo del tuo conto corrente bancario",
		},
		{
			Name:                     "visualizza",
			Description:              "Comandi di visualizzazione database per lo Staff",
			DefaultMemberPermissions: &staffPerm,
			Options: []*discordgo.ApplicationCommandOption{
				setupSub("anagrafe", "Visualizza tutte le carte d'identità registrate"),
				setupSub("porto-darmi", "Visualizza tutte le richieste di porto d'armi"),
				setupSub("immatricolazioni", "Visualizza tutti i veicoli immatricolati"),
			},
		},
		{
			Name:                     "setup",
			Description:              "Configura i pannelli interattivi e i canali del server",
			DefaultMemberPermissions: &staffPerm,
			Options: []*discordgo.ApplicationCommandOption{
				setupSub("server", "Pannello generale per configurare canali (#) e ruoli (@) di ogni comando"),
				setupSub("apri-conto", "Invia il pannello bancario per l'apertura del conto"),
				setupSub("busta-paga", fmt.Sprintf("Invia il pannello per la busta paga settimanale (€ %d)", stipendioFisso)),
				setupSub("cittadinanza", "Invia il pannello anagrafe per la carta d'identità"),
				setupSub("porto-darmi", "Invia il pannello per la richiesta del porto d'armi"),
				setupSub("immatricolazione", "Invia il pannello PRA per immatricolare i veicoli"),
				setupSub("registra-arresto", "Invia il pannello delle Forze dell'Ordine per registrare arresti"),
				setupSub("emetti-multa", "Invia il pannello per la sanzione stradale (verbale multa)"),
				setupSub("tasse", "Gestisci e invia il pannello tasse (conto, immatricolazione, immobili)"),
				setupSub("staff-panel", "Invia il pannello riservato alla moderazione Staff"),
			},
		},
	}

	// Cancella i comandi GLOBALI vecchi (causa dei comandi doppi)
	if _, err := s.ApplicationCommandBulkOverwrite(s.State.User.ID, "", []*discordgo.ApplicationCommand{}); err != nil {
		log.Printf("Errore pulizia comandi globali: %v", err)
	}

	// Sovrascrive tutti i comandi del server: niente comandi vecchi rimasti in giro
	_, err := s.ApplicationCommandBulkOverwrite(s.State.User.ID, guildID, commands)
	if err != nil {
		log.Printf("Errore nella registrazione dei comandi: %v", err)
	}
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

	if data.Name == "conto" {
		userID := i.Member.User.ID
		account, exists := userBankAccounts[userID]
		if !exists {
			respond(s, i, "❌ Non possiedi ancora un conto corrente registrato! Usa il pannello di apertura conto per aprirne uno.")
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
				"**Utente Discord:** <@%s>\n**Intestatario RP:** %s\n**Utente Roblox:** `@%s`\n**Istituto Bancario:** %s\n💰 **Saldo Disponibile:** `€ %d`\n\n"+
					"🧾 **Le tue tasse (%s):**\n• Conto: € %d\n• Immatricolazione: € %d\n• Immobili: € %d\n**Totale:** € %d",
				userID, account.OwnerName, account.RobloxName, strings.ToUpper(account.BankName), account.Balance,
				tipo, t.Conto, t.Immatricolazione, t.Immobili, t.Conto+t.Immatricolazione+t.Immobili),
		}
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{embed},
				Flags:  discordgo.MessageFlagsEphemeral,
			},
		})
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
			sb.WriteString("📋 **LISTA CITTADINANZA / ANAGRAFE REGISTRATA:**\n\n")
			if len(dbAnagrafe) == 0 {
				sb.WriteString("*Nessuna carta d'identità registrata.*")
			}
			for _, rec := range dbAnagrafe {
				sb.WriteString(fmt.Sprintf("• <@%s> | **RP:** %s %s | **Roblox:** `@%s` | **Nato:** %s\n", rec.UserID, rec.NomeRP, rec.CognomeRP, rec.RobloxUser, rec.DataNascita))
			}
		case "porto-darmi":
			sb.WriteString("🔫 **LISTA PORTO D'ARMI RICHIESTI:**\n\n")
			if len(dbPortoDarmi) == 0 {
				sb.WriteString("*Nessuna richiesta di porto d'armi.*")
			}
			for _, rec := range dbPortoDarmi {
				sb.WriteString(fmt.Sprintf("• <@%s> | **RP:** %s | **Roblox:** `@%s` | **Motivazione:** %s\n", rec.UserID, rec.NomeRP, rec.RobloxUser, rec.Motivazione))
			}
		case "immatricolazioni":
			sb.WriteString("🚗 **LISTA VEICOLI IMMATRICOLATI (PRA):**\n\n")
			if len(dbImmatricolazioni) == 0 {
				sb.WriteString("*Nessun veicolo immatricolato.*")
			}
			for _, rec := range dbImmatricolazioni {
				sb.WriteString(fmt.Sprintf("• <@%s> | **Modello:** %s | **Targa:** `%s` | **Roblox:** `@%s`\n", rec.UserID, rec.ModelloAuto, rec.Targa, rec.RobloxUser))
			}
		}
		respond(s, i, truncate(sb.String(), 1900))
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
// PANNELLI DI SETUP
// ---------------------------------------------------------------------------

func channelInfoText(key string) string {
	cfg := getOrCreateConfig(key)
	if cfg.PanelChannelID == "" {
		return "⚠️ *Nessun canale impostato in `/setup server`. Verrà inviato in questo canale.*"
	}
	return fmt.Sprintf("📍 *Canale di destinazione:* <#%s>", cfg.PanelChannelID)
}

func sendTasseSetupPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title: "🏛️ GESTIONE FISCALE E TASSE - ROME ITALY 2.0",
		Description: fmt.Sprintf(
			"Usa i pulsanti per pubblicare il pannello o configurare le tasse.\n\n"+
				"**Tasse generali attuali:**\n• Conto: € %d\n• Immatricolazione: € %d\n• Immobili: € %d\n• Frequenza: ogni %d giorni (%s)\n\n"+
				"Utenti con tasse personalizzate: **%d**\n\n%s",
			defaultTaxes.Conto, defaultTaxes.Immatricolazione, defaultTaxes.Immobili,
			defaultTaxes.IntervalDays, defaultTaxes.CustomDeadline,
			len(userTaxOverrides), channelInfoText("tasse")),
		Color: 0x95A5A6,
	}

	row1 := discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{Label: "Invia Pannello nel Canale", Style: discordgo.SuccessButton, CustomID: "send_panel_to_channel_tasse", Emoji: &discordgo.ComponentEmoji{Name: "📨"}},
		discordgo.Button{Label: "Tasse Generali", Style: discordgo.PrimaryButton, CustomID: "btn_config_tasse_generali", Emoji: &discordgo.ComponentEmoji{Name: "⚙️"}},
		discordgo.Button{Label: "Tempo / Frequenza", Style: discordgo.PrimaryButton, CustomID: "btn_config_tasse_time", Emoji: &discordgo.ComponentEmoji{Name: "⏱️"}},
	}}
	row2 := discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{Label: "Tassa Personalizzata Utente", Style: discordgo.SecondaryButton, CustomID: "btn_config_tasse_user", Emoji: &discordgo.ComponentEmoji{Name: "👤"}},
		discordgo.Button{Label: "Rimuovi Tassa Personalizzata", Style: discordgo.DangerButton, CustomID: "btn_reset_tasse_user", Emoji: &discordgo.ComponentEmoji{Name: "🗑️"}},
	}}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: []discordgo.MessageComponent{row1, row2},
			Flags:      discordgo.MessageFlagsEphemeral,
		},
	})
}

func sendPanelPreview(s *discordgo.Session, i *discordgo.InteractionCreate, serviceKey string) {
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("📤 INVIA PANNELLO: %s", strings.ToUpper(serviceKey)),
		Description: fmt.Sprintf("Clicca sul pulsante per pubblicare ufficialmente il pannello nel canale configurato.\n\n%s", channelInfoText(serviceKey)),
		Color:       0x3498DB,
	}
	btnSend := discordgo.Button{
		Label:    "Invia Pannello nel Canale",
		Style:    discordgo.SuccessButton,
		CustomID: "send_panel_to_channel_" + serviceKey,
		Emoji:    &discordgo.ComponentEmoji{Name: "📨"},
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{btnSend}},
			},
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
}

func sendServerSetupPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "⚙️ CONFIGURAZIONE GLOBALE COMANDI - ROME ITALY 2.0",
		Description: "Seleziona dal menu il comando che vuoi configurare per impostare canali e ruoli autorizzati:",
		Color:       0x0099FF,
	}

	opt := func(label, value, emoji string) discordgo.SelectMenuOption {
		return discordgo.SelectMenuOption{
			Label:       label,
			Value:       value,
			Description: "Imposta canali e ruoli",
			Emoji:       &discordgo.ComponentEmoji{Name: emoji},
		}
	}

	selectMenu := discordgo.SelectMenu{
		CustomID:    "setup_server_select",
		Placeholder: "Scegli il comando da configurare...",
		Options: []discordgo.SelectMenuOption{
			opt("Cittadinanza & Anagrafe", "cittadinanza", "🇮🇹"),
			opt("Porto d'Armi", "porto-darmi", "🔫"),
			opt("Apertura Conto Bancario", "apri-conto", "💳"),
			opt("Riscossione Busta Paga", "busta-paga", "💵"),
			opt("Immatricolazione Veicoli", "immatricolazione", "🚗"),
			opt("Registro Arresti FDO", "registra-arresto", "🚨"),
			opt("Emissione Multe", "emetti-multa", "📄"),
			opt("Tasse e Fisco", "tasse", "🏛️"),
			opt("Staff & Moderazione Panel", "staff-panel", "🛡️"),
		},
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{selectMenu}},
			},
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
}

// ---------------------------------------------------------------------------
// COMPONENTI (bottoni e menu)
// ---------------------------------------------------------------------------

func handleComponents(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	values := i.MessageComponentData().Values

	if i.Member == nil {
		return
	}

	// --- Invio pannello pubblico ---
	if strings.HasPrefix(customID, "send_panel_to_channel_") {
		serviceKey := strings.TrimPrefix(customID, "send_panel_to_channel_")
		targetChannel := getOrCreateConfig(serviceKey).PanelChannelID
		if targetChannel == "" {
			targetChannel = i.ChannelID
		}
		publishPublicPanel(s, targetChannel, serviceKey)
		respond(s, i, fmt.Sprintf("✅ **Pannello inviato con successo nel canale <#%s>!**", targetChannel))
		return
	}

	// --- Configurazione tasse ---
	switch customID {
	case "btn_config_tasse_generali":
		openModal(s, i, "modal_config_tasse_base", "Configura Tasse Generali",
			inputRow("input_tax_conto", "Tassa Conto Corrente (€)", discordgo.TextInputShort, strconv.Itoa(defaultTaxes.Conto), "", true),
			inputRow("input_tax_imm", "Tassa Immatricolazione (€)", discordgo.TextInputShort, strconv.Itoa(defaultTaxes.Immatricolazione), "", true),
			inputRow("input_tax_immobili", "Tassa Immobili (€)", discordgo.TextInputShort, strconv.Itoa(defaultTaxes.Immobili), "", true),
		)
		return

	case "btn_config_tasse_time":
		openModal(s, i, "modal_config_tasse_time", "Tempo e Scadenza Tasse",
			inputRow("input_tax_days", "Ogni quanti giorni pagare le tasse?", discordgo.TextInputShort, strconv.Itoa(defaultTaxes.IntervalDays), "Es. 7", true),
			inputRow("input_tax_deadline_note", "Note/Scadenza", discordgo.TextInputShort, defaultTaxes.CustomDeadline, "Es. Entro la domenica", true),
		)
		return

	case "btn_config_tasse_user", "btn_reset_tasse_user":
		prefix, text := "seluser_tasse", "👤 Scegli l'utente a cui impostare le tasse personalizzate (immatricolazione, conto, immobili):"
		if customID == "btn_reset_tasse_user" {
			prefix, text = "seluserreset_tasse", "🗑️ Scegli l'utente a cui rimuovere la tassa personalizzata (tornerà alle tasse generali):"
		}
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: text,
				Flags:   discordgo.MessageFlagsEphemeral,
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{CustomID: prefix, Placeholder: "Seleziona utente...", MenuType: discordgo.UserSelectMenu},
					}},
				},
			},
		})
		return

	case "seluser_tasse":
		if len(values) == 0 {
			return
		}
		target := values[0]
		t, _ := getUserTaxes(target)
		openModal(s, i, "modal_tasse_user_"+target, "Tasse Personalizzate Utente",
			inputRow("u_conto", "Tassa Conto (€)", discordgo.TextInputShort, strconv.Itoa(t.Conto), "", true),
			inputRow("u_imm", "Tassa Immatricolazione (€)", discordgo.TextInputShort, strconv.Itoa(t.Immatricolazione), "", true),
			inputRow("u_immobili", "Tassa Immobili (€)", discordgo.TextInputShort, strconv.Itoa(t.Immobili), "", true),
		)
		return

	case "seluserreset_tasse":
		if len(values) == 0 {
			return
		}
		target := values[0]
		if _, ok := userTaxOverrides[target]; ok {
			delete(userTaxOverrides, target)
			respond(s, i, fmt.Sprintf("✅ Tassa personalizzata rimossa per <@%s>. Ora paga le tasse generali.", target))
		} else {
			respond(s, i, fmt.Sprintf("ℹ️ <@%s> non aveva tasse personalizzate.", target))
		}
		return
	}

	// --- Approvazioni Staff ---
	if strings.HasPrefix(customID, "approve_yes_") || strings.HasPrefix(customID, "approve_no_") {
		parts := strings.Split(customID, "_")
		if len(parts) < 3 || len(i.Message.Embeds) == 0 {
			return
		}
		azione := parts[1]
		targetUserID := parts[2]

		newEmbed := *i.Message.Embeds[0]
		if azione == "yes" {
			newEmbed.Color = 0x2ECC71
			newEmbed.Title = "✅ APPROVATO DALLO STAFF"
			newEmbed.Description += fmt.Sprintf("\n\n**Stato:** Approvato da <@%s>", i.Member.User.ID)
		} else {
			newEmbed.Color = 0xE74C3C
			newEmbed.Title = "❌ RIFIUTATO DALLO STAFF"
			newEmbed.Description += fmt.Sprintf("\n\n**Stato:** Rifiutato da <@%s>", i.Member.User.ID)
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Embeds:     []*discordgo.MessageEmbed{&newEmbed},
				Components: []discordgo.MessageComponent{},
			},
		})

		if ch, err := s.UserChannelCreate(targetUserID); err == nil {
			if azione == "yes" {
				s.ChannelMessageSend(ch.ID, "🎉 Salve, la tua richiesta inviata nel server è stata **APPROVATA** dallo staff!")
			} else {
				s.ChannelMessageSend(ch.ID, "❌ Salve, la tua richiesta inviata nel server è stata **RIFIUTATA** dallo staff.")
			}
		}
		return
	}

	// --- Setup server: scelta comando ---
	if customID == "setup_server_select" {
		selected := "cittadinanza"
		if len(values) > 0 {
			selected = values[0]
		}

		chanTypes := []discordgo.ChannelType{discordgo.ChannelTypeGuildText}
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("🛠️ **Configurazione per: %s**\nScegli i canali (`#`) e il ruolo (`@`):", strings.ToUpper(selected)),
				Flags:   discordgo.MessageFlagsEphemeral,
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{CustomID: "selchanpanel_" + selected, Placeholder: "Canale del pannello (#)...", MenuType: discordgo.ChannelSelectMenu, ChannelTypes: chanTypes},
					}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{CustomID: "selchanesiti_" + selected, Placeholder: "Canale dei risultati/esiti (#)...", MenuType: discordgo.ChannelSelectMenu, ChannelTypes: chanTypes},
					}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{CustomID: "selroleauth_" + selected, Placeholder: "Ruolo autorizzato (@)...", MenuType: discordgo.RoleSelectMenu},
					}},
				},
			},
		})
		return
	}

	// --- Setup server: salvataggio canali/ruolo ---
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
		respond(s, i, "✅ **Selezione salvata con successo!**")
		return
	}

	// --- Bottoni dei pannelli pubblici ---
	switch customID {
	case "btn_paga_tasse":
		userID := i.Member.User.ID
		account, exists := userBankAccounts[userID]
		if !exists {
			respond(s, i, "❌ Devi prima aprire un conto corrente bancario per poter versare le tasse!")
			return
		}

		interval := time.Duration(defaultTaxes.IntervalDays) * 24 * time.Hour
		if interval > 0 && !account.LastTaxPaid.IsZero() && time.Since(account.LastTaxPaid) < interval {
			rim := interval - time.Since(account.LastTaxPaid)
			giorni := int(rim.Hours()) / 24
			ore := int(rim.Hours()) % 24
			respond(s, i, fmt.Sprintf("⏳ **Hai già saldato le tasse per questo periodo!**\nPotrai versarle di nuovo tra **%d giorni e %d ore**.\n📌 *Scadenza:* %s", giorni, ore, defaultTaxes.CustomDeadline))
			return
		}

		t, personalized := getUserTaxes(userID)
		totale := t.Conto + t.Immatricolazione + t.Immobili

		if account.Balance < totale {
			respond(s, i, fmt.Sprintf("⚠️ **Saldo insufficiente!** Tasse da saldare: **€ %d**, saldo attuale: `€ %d`.", totale, account.Balance))
			return
		}

		account.Balance -= totale
		account.LastTaxPaid = time.Now()
		userBankAccounts[userID] = account

		tipo := "Generale Standard"
		if personalized {
			tipo = "Personalizzata"
		}

		respond(s, i, fmt.Sprintf("🏛️ **Tasse Versate con Successo!**\n• Conto: € %d\n• Immatricolazione: € %d\n• Immobili: € %d\n💰 **Totale Detratto:** `€ %d` | Nuovo Saldo: `€ %d`",
			t.Conto, t.Immatricolazione, t.Immobili, totale, account.Balance))

		embedEsito := &discordgo.MessageEmbed{
			Title: "🏛️ RICEVUTA PAGAMENTO TASSE STATALI",
			Color: 0x2ECC71,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Cittadino Discord", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
				{Name: "Tipo Aliquota", Value: tipo, Inline: true},
				{Name: "Tassa Conto", Value: fmt.Sprintf("€ %d", t.Conto), Inline: true},
				{Name: "Tassa Immatricolazione", Value: fmt.Sprintf("€ %d", t.Immatricolazione), Inline: true},
				{Name: "Tassa Immobili", Value: fmt.Sprintf("€ %d", t.Immobili), Inline: true},
				{Name: "Totale Versato", Value: fmt.Sprintf("`€ %d`", totale), Inline: true},
			},
		}
		s.ChannelMessageSendComplex(esitiChannel("tasse", i.ChannelID), &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{embedEsito}})

	case "btn_apri_conto":
		if _, exists := userBankAccounts[i.Member.User.ID]; exists {
			respond(s, i, "❌ Hai già un conto corrente aperto! Usa `/conto` per vedere il saldo.")
			return
		}
		selectBanche := discordgo.SelectMenu{
			CustomID:    "select_banca_choice",
			Placeholder: "Seleziona il tuo istituto bancario...",
			Options: []discordgo.SelectMenuOption{
				{Label: "Banca Sella", Value: "banca_sella", Emoji: &discordgo.ComponentEmoji{Name: "🏛️"}},
				{Label: "Poste Italiane", Value: "poste", Emoji: &discordgo.ComponentEmoji{Name: "📮"}},
				{Label: "Intesa Sanpaolo", Value: "intesa", Emoji: &discordgo.ComponentEmoji{Name: "🏦"}},
				{Label: "UniCredit", Value: "unicredit", Emoji: &discordgo.ComponentEmoji{Name: "🔴"}},
				{Label: "BPER Banca", Value: "bper", Emoji: &discordgo.ComponentEmoji{Name: "🟦"}},
				{Label: "Banco BPM", Value: "bpm", Emoji: &discordgo.ComponentEmoji{Name: "🟢"}},
				{Label: "Revolut", Value: "revolut", Emoji: &discordgo.ComponentEmoji{Name: "💳"}},
			},
		}
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Scegli la banca con cui desideri aprire il conto corrente (riceverai subito **€ 7.000** di bonus):",
				Flags:   discordgo.MessageFlagsEphemeral,
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{selectBanche}},
				},
			},
		})

	case "select_banca_choice":
		selected := "banca_generica"
		if len(values) > 0 {
			selected = values[0]
		}
		openModal(s, i, "modal_conferma_conto_"+selected, "Apertura Conto Corrente",
			inputRow("input_nome_cognome_rp", "Nome e Cognome RP", discordgo.TextInputShort, "", "Es. Mario Rossi", true),
			inputRow("input_roblox_username", "Username Roblox", discordgo.TextInputShort, "", "Es. RobloxUser123", true),
		)

	case "btn_richiedi_cittadinanza":
		openModal(s, i, "modal_cittadinanza", "Registrazione Carta d'Identità",
			inputRow("input_nome_rp", "Nome RP", discordgo.TextInputShort, "", "Es. Mario", true),
			inputRow("input_cognome_rp", "Cognome RP", discordgo.TextInputShort, "", "Es. Rossi", true),
			inputRow("input_roblox_user", "Username Roblox", discordgo.TextInputShort, "", "Es. RobloxPlayer1", true),
			inputRow("input_data_nascita", "Data di Nascita RP", discordgo.TextInputShort, "", "GG/MM/AAAA", true),
		)

	case "btn_richiedi_porto_darmi":
		openModal(s, i, "modal_porto_darmi", "Richiesta Licenza Porto d'Armi",
			inputRow("input_nome_cognome_pda", "Nome e Cognome RP", discordgo.TextInputShort, "", "", true),
			inputRow("input_roblox_pda", "Username Roblox", discordgo.TextInputShort, "", "", true),
			inputRow("input_motivazione_pda", "Motivazione Richiesta", discordgo.TextInputParagraph, "", "", true),
		)

	case "btn_immatricolazione":
		openModal(s, i, "modal_immatricolazione", "Immatricolazione Veicolo (PRA)",
			inputRow("input_roblox_imm", "Username Roblox", discordgo.TextInputShort, "", "", true),
			inputRow("input_modello_auto", "Modello Veicolo", discordgo.TextInputShort, "", "", true),
			inputRow("input_targa", "Targa Desiderata", discordgo.TextInputShort, "", "", true),
		)

	case "btn_registra_arresto":
		openModal(s, i, "modal_registra_arresto", "Verbale d'Arresto",
			inputRow("input_utente_arrestato", "Utente Roblox Arrestato", discordgo.TextInputShort, "", "", true),
			inputRow("input_reati", "Reati Commessi", discordgo.TextInputParagraph, "", "Inseguimento...", true),
			inputRow("input_prigione", "Anni o Mesi di Prigione", discordgo.TextInputShort, "", "", true),
			inputRow("input_cauzione", "Cauzione In Euro", discordgo.TextInputShort, "", "5000", false),
		)

	case "btn_emetti_multa":
		openModal(s, i, "modal_emetti_multa", "Sanzione Stradale / Verbale Multa",
			inputRow("input_utente_multato", "Utente Roblox Multato", discordgo.TextInputShort, "", "", true),
			inputRow("input_importo_multa", "Importo Multa in Euro (€)", discordgo.TextInputShort, "", "150", true),
			inputRow("input_motivo_multa", "Motivazione / Infrazione", discordgo.TextInputParagraph, "", "Eccesso di velocità...", true),
			inputRow("input_targa_multa", "Targa Veicolo (opzionale)", discordgo.TextInputShort, "", "", false),
		)

	case "btn_riscuoti_stipendio":
		userID := i.Member.User.ID
		account, exists := userBankAccounts[userID]
		if !exists {
			respond(s, i, "❌ Devi prima aprire un conto corrente bancario per poter riscuotere la busta paga!")
			return
		}

		week := 7 * 24 * time.Hour
		if !account.LastPayday.IsZero() && time.Since(account.LastPayday) < week {
			rim := week - time.Since(account.LastPayday)
			giorni := int(rim.Hours()) / 24
			ore := int(rim.Hours()) % 24
			respond(s, i, fmt.Sprintf("⏳ **Hai già riscosso la busta paga questa settimana!** Potrai riscuoterla di nuovo tra **%d giorni e %d ore**.", giorni, ore))
			return
		}

		account.Balance += stipendioFisso
		account.LastPayday = time.Now()
		userBankAccounts[userID] = account

		respond(s, i, fmt.Sprintf("✅ **Busta Paga Riscossa con Successo!** Ti sono stati accreditati **€ %d** sul tuo conto. Saldo totale attuale: `€ %d`", stipendioFisso, account.Balance))
	}
}

// ---------------------------------------------------------------------------
// PUBBLICAZIONE PANNELLI
// ---------------------------------------------------------------------------

func publishPublicPanel(s *discordgo.Session, channelID string, serviceKey string) {
	switch serviceKey {
	case "apri-conto":
		sendPanel(s, channelID, &discordgo.MessageEmbed{
			Title:       "🏛️ APERTURA CONTO CORRENTE - ROME ITALY 2.0",
			Description: "Clicca sul bottone qui sotto per scegliere il tuo istituto bancario e ricevere **€ 7.000** di bonus benvenuto!",
			Color:       0x2ECC71,
		}, &discordgo.Button{Label: "Apri Conto Corrente", Style: discordgo.SuccessButton, CustomID: "btn_apri_conto", Emoji: &discordgo.ComponentEmoji{Name: "💳"}})

	case "porto-darmi":
		sendPanel(s, channelID, &discordgo.MessageEmbed{
			Title:       "🔫 RICHIESTA PORTO D'ARMI - ROME ITALY 2.0",
			Description: "Clicca sul bottone per inviare la richiesta di licenza porto d'armi alle autorità competenti.",
			Color:       0xE67E22,
		}, &discordgo.Button{Label: "Richiedi Porto d'Armi", Style: discordgo.PrimaryButton, CustomID: "btn_richiedi_porto_darmi", Emoji: &discordgo.ComponentEmoji{Name: "📜"}})

	case "busta-paga":
		sendPanel(s, channelID, &discordgo.MessageEmbed{
			Title:       "💼 RISCOSSIONE BUSTA PAGA - ROME ITALY 2.0",
			Description: fmt.Sprintf("Clicca sul bottone qui sotto per riscuotere la tua busta paga settimanale di **€ %d** sul tuo conto corrente!", stipendioFisso),
			Color:       0xF1C40F,
		}, &discordgo.Button{Label: fmt.Sprintf("Riscuoti Busta Paga (€ %d)", stipendioFisso), Style: discordgo.PrimaryButton, CustomID: "btn_riscuoti_stipendio", Emoji: &discordgo.ComponentEmoji{Name: "💵"}})

	case "cittadinanza":
		sendPanel(s, channelID, &discordgo.MessageEmbed{
			Title:       "🇮🇹 ANAGRAFE CENTRALE - ROME ITALY 2.0",
			Description: "Richiedi la tua Carta d'Identità e la Cittadinanza ufficiali.",
			Color:       0x3498DB,
		}, &discordgo.Button{Label: "Richiedi Cittadinanza", Style: discordgo.PrimaryButton, CustomID: "btn_richiedi_cittadinanza", Emoji: &discordgo.ComponentEmoji{Name: "🆔"}})

	case "immatricolazione":
		sendPanel(s, channelID, &discordgo.MessageEmbed{
			Title:       "🚗 UFFICIO PRA - IMMATRICOLAZIONE VEICOLI",
			Description: "Clicca sul bottone per registrare e immatricolare ufficialmente il tuo veicolo nel server.",
			Color:       0x9B59B6,
		}, &discordgo.Button{Label: "Immatricola Veicolo", Style: discordgo.PrimaryButton, CustomID: "btn_immatricolazione", Emoji: &discordgo.ComponentEmoji{Name: "🚘"}})

	case "registra-arresto":
		sendPanel(s, channelID, &discordgo.MessageEmbed{
			Title:       "🚨 BANCA DATI POLIZIA DI STATO / CARABINIERI",
			Description: "Compila il verbale dei fermi e degli arresti eseguiti durante il servizio.",
			Color:       0xE74C3C,
		}, &discordgo.Button{Label: "Registra Arresto", Style: discordgo.DangerButton, CustomID: "btn_registra_arresto", Emoji: &discordgo.ComponentEmoji{Name: "📝"}})

	case "emetti-multa":
		sendPanel(s, channelID, &discordgo.MessageEmbed{
			Title:       "📄 POLIZIA LOCALE - EMISSIONE VERBALE MULTA",
			Description: "Clicca sul bottone per emettere una sanzione stradale nei confronti di un cittadino.",
			Color:       0xE74C3C,
		}, &discordgo.Button{Label: "Emetti Multa", Style: discordgo.DangerButton, CustomID: "btn_emetti_multa", Emoji: &discordgo.ComponentEmoji{Name: "📄"}})

	case "tasse":
		sendPanel(s, channelID, &discordgo.MessageEmbed{
			Title: "🏛️ AGENZIA DELLE ENTRATE - PAGAMENTO TASSE STATALI",
			Description: fmt.Sprintf("Clicca sul bottone per versare le tue imposte: **conto**, **immatricolazione** e **immobili**.\nOgni cittadino paga l'importo a lui assegnato (controlla con `/conto`).\n\n📌 **Scadenza / Frequenza:** %s", defaultTaxes.CustomDeadline),
			Color:       0x95A5A6,
		}, &discordgo.Button{Label: "Paga Tasse Statali", Style: discordgo.SuccessButton, CustomID: "btn_paga_tasse", Emoji: &discordgo.ComponentEmoji{Name: "💸"}})

	case "staff-panel":
		sendPanel(s, channelID, &discordgo.MessageEmbed{
			Title:       "🛡️ PANNELLO STAFF - ROME ITALY 2.0",
			Description: "Comandi riservati allo Staff:\n• `/visualizza anagrafe`\n• `/visualizza porto-darmi`\n• `/visualizza immatricolazioni`\n• `/setup tasse` per gestire le tasse",
			Color:       0x2C3E50,
		}, nil)
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

	// --- Tasse generali ---
	if customID == "modal_config_tasse_base" {
		defaultTaxes.Conto = parseEuro(modalVal(data, 0))
		defaultTaxes.Immatricolazione = parseEuro(modalVal(data, 1))
		defaultTaxes.Immobili = parseEuro(modalVal(data, 2))
		respond(s, i, fmt.Sprintf("✅ **Tasse Generali Aggiornate!**\n• Conto: € %d\n• Immatricolazione: € %d\n• Immobili: € %d",
			defaultTaxes.Conto, defaultTaxes.Immatricolazione, defaultTaxes.Immobili))
		return
	}

	// --- Tempo tasse ---
	if customID == "modal_config_tasse_time" {
		days := parseEuro(modalVal(data, 0))
		if days < 1 {
			days = 1
		}
		defaultTaxes.IntervalDays = days
		defaultTaxes.CustomDeadline = modalVal(data, 1)
		respond(s, i, fmt.Sprintf("✅ **Tempo Tasse Aggiornato!**\n• Frequenza: ogni **%d giorni**\n• Note scadenza: *%s*", defaultTaxes.IntervalDays, defaultTaxes.CustomDeadline))
		return
	}

	// --- Tasse personalizzate per utente ---
	if strings.HasPrefix(customID, "modal_tasse_user_") {
		target := strings.TrimPrefix(customID, "modal_tasse_user_")
		t := UserTaxOverride{
			Conto:            parseEuro(modalVal(data, 0)),
			Immatricolazione: parseEuro(modalVal(data, 1)),
			Immobili:         parseEuro(modalVal(data, 2)),
		}
		userTaxOverrides[target] = t
		respond(s, i, fmt.Sprintf("✅ **Tasse personalizzate per <@%s>!**\n• Conto: € %d\n• Immatricolazione: € %d\n• Immobili: € %d\n**Totale:** € %d",
			target, t.Conto, t.Immatricolazione, t.Immobili, t.Conto+t.Immatricolazione+t.Immobili))
		return
	}

	// --- Apertura conto ---
	if strings.HasPrefix(customID, "modal_conferma_conto_") {
		banca := strings.TrimPrefix(customID, "modal_conferma_conto_")
		nomeRP := modalVal(data, 0)
		robloxUser := modalVal(data, 1)

		if _, exists := userBankAccounts[userID]; exists {
			respond(s, i, "❌ Hai già un conto corrente aperto!")
			return
		}

		userBankAccounts[userID] = BankAccount{
			OwnerName:  nomeRP,
			RobloxName: robloxUser,
			BankName:   banca,
			Balance:    7000,
		}

		respond(s, i, "🎉 **Conto Corrente Aperto con Successo!** Ti sono stati accreditati **€ 7.000** di bonus benvenuto. Verifica il saldo con `/conto`.")

		embed := &discordgo.MessageEmbed{
			Title: "💳 NUOVA APERTURA CONTO BANCARIO",
			Color: 0x2ECC71,
			Description: fmt.Sprintf("**Richiedente Discord:** <@%s>\n**Utente Roblox:** `@%s`\n**Nome & Cognome RP:** %s\n**Banca:** %s\n\n*Saldo accreditato:* € 7.000",
				userID, robloxUser, nomeRP, strings.ToUpper(banca)),
		}
		sendApproval(s, esitiChannel("apri-conto", i.ChannelID), embed, userID, "Accetta (Sì)", "Rifiuta (No)")
		return
	}

	switch customID {
	case "modal_porto_darmi":
		nomeRP := modalVal(data, 0)
		robloxUser := modalVal(data, 1)
		motivazione := modalVal(data, 2)

		dbPortoDarmi[userID] = PortoDarmiRecord{UserID: userID, RobloxUser: robloxUser, NomeRP: nomeRP, Motivazione: motivazione}

		respond(s, i, "📜 **Richiesta Porto d'Armi Inviata!** Sarà valutata dalle autorità.")

		embed := &discordgo.MessageEmbed{
			Title: "🔫 RICHIESTA LICENZA PORTO D'ARMI",
			Color: 0xE67E22,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Richiedente Discord", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
				{Name: "Utente Roblox", Value: fmt.Sprintf("`@%s`", robloxUser), Inline: true},
				{Name: "Nome & Cognome RP", Value: nomeRP, Inline: true},
				{Name: "Motivazione", Value: truncate(motivazione, 1000), Inline: false},
			},
		}
		sendApproval(s, esitiChannel("porto-darmi", i.ChannelID), embed, userID, "Accetta (Sì)", "Rifiuta (No)")

	case "modal_cittadinanza":
		nomeRP := modalVal(data, 0)
		cognomeRP := modalVal(data, 1)
		robloxUser := modalVal(data, 2)
		dataNascita := modalVal(data, 3)

		dbAnagrafe[userID] = AnagrafeRecord{UserID: userID, RobloxUser: robloxUser, NomeRP: nomeRP, CognomeRP: cognomeRP, DataNascita: dataNascita}

		respond(s, i, "🇮🇹 **Richiesta Cittadinanza Registrata!** Inviata nel canale esiti.")

		embed := &discordgo.MessageEmbed{
			Title: "🇮🇹 NUOVA RICHIESTA CITTADINANZA / CARTA D'IDENTITÀ",
			Color: 0x3498DB,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Richiedente Discord", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
				{Name: "Utente Roblox", Value: fmt.Sprintf("`@%s`", robloxUser), Inline: true},
				{Name: "Nome RP", Value: nomeRP, Inline: true},
				{Name: "Cognome RP", Value: cognomeRP, Inline: true},
				{Name: "Data di Nascita", Value: dataNascita, Inline: true},
			},
		}
		sendApproval(s, esitiChannel("cittadinanza", i.ChannelID), embed, userID, "Accetta (Sì)", "Rifiuta (No)")

	case "modal_immatricolazione":
		robloxUser := modalVal(data, 0)
		modelloAuto := modalVal(data, 1)
		targa := strings.ToUpper(strings.TrimSpace(modalVal(data, 2)))

		if existing, taken := dbImmatricolazioni[targa]; taken && existing.UserID != userID {
			respond(s, i, fmt.Sprintf("❌ La targa `%s` è già registrata a un altro cittadino. Scegline un'altra.", targa))
			return
		}

		dbImmatricolazioni[targa] = ImmatricolazioneRecord{UserID: userID, RobloxUser: robloxUser, Targa: targa, ModelloAuto: modelloAuto}

		respond(s, i, "🚗 **Immatricolazione Inviata!** Inserita nel registro PRA.")

		embed := &discordgo.MessageEmbed{
			Title: "🚗 NUOVA RICHIESTA IMMATRICOLAZIONE VEICOLO",
			Color: 0x9B59B6,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Richiedente Discord", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
				{Name: "Utente Roblox", Value: fmt.Sprintf("`@%s`", robloxUser), Inline: true},
				{Name: "Modello Veicolo", Value: modelloAuto, Inline: true},
				{Name: "Targa Richiesta", Value: fmt.Sprintf("`%s`", targa), Inline: true},
			},
		}
		sendApproval(s, esitiChannel("immatricolazione", i.ChannelID), embed, userID, "Accetta (Sì)", "Rifiuta (No)")

	case "modal_registra_arresto":
		utenteArrestato := modalVal(data, 0)
		reati := modalVal(data, 1)
		prigione := modalVal(data, 2)
		cauzione := modalVal(data, 3)
		if cauzione == "" {
			cauzione = "Nessuna"
		}

		embed := &discordgo.MessageEmbed{
			Title: "🚨 VERBALE D'ARRESTO UFFICIALE",
			Color: 0xE74C3C,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Agente Discord", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
				{Name: "Arrestato Roblox", Value: fmt.Sprintf("`@%s`", utenteArrestato), Inline: true},
				{Name: "Pena", Value: prigione, Inline: true},
				{Name: "Cauzione", Value: cauzione, Inline: true},
				{Name: "Reati", Value: truncate(reati, 1000), Inline: false},
			},
		}

		respond(s, i, "🚨 **Verbale Registrato!** Inviato nel canale del registro ufficiale.")
		sendApproval(s, esitiChannel("registra-arresto", i.ChannelID), embed, userID, "Approva Verbale (Sì)", "Annulla Verbale (No)")

	case "modal_emetti_multa":
		utenteMultato := modalVal(data, 0)
		importo := modalVal(data, 1)
		motivo := modalVal(data, 2)
		targa := modalVal(data, 3)
		if targa == "" {
			targa = "N/D"
		}

		embed := &discordgo.MessageEmbed{
			Title: "📄 SANZIONE STRADALE / VERBALE MULTA",
			Color: 0xE74C3C,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Agente Discord", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
				{Name: "Cittadino Multato (Roblox)", Value: fmt.Sprintf("`@%s`", utenteMultato), Inline: true},
				{Name: "Importo Multa", Value: fmt.Sprintf("€ %s", importo), Inline: true},
				{Name: "Targa Veicolo", Value: fmt.Sprintf("`%s`", targa), Inline: true},
				{Name: "Infrazione / Motivazione", Value: truncate(motivo, 1000), Inline: false},
			},
		}

		respond(s, i, "📄 **Verbale di Multa Emesso e Registrato con Successo!**")
		sendApproval(s, esitiChannel("emetti-multa", i.ChannelID), embed, userID, "Approva Multa (Sì)", "Annulla Multa (No)")
	}
}
