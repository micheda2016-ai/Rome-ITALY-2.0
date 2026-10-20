package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
)

// Struttura per salvare in memoria i canali e i ruoli configurati
type ServerConfig struct {
	PanelChannelID string
	EsitiChannelID string
	AuthRoleID     string
}

// Mappa per memorizzare le configurazioni di ogni comando
var serverConfigs = make(map[string]ServerConfig)

func main() {
	// Server HTTP per Render (mantiene il bot sempre attivo sulla porta 10000)
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

	// Recupera e pulisce il token Discord
	token := strings.TrimSpace(os.Getenv("DISCORD_TOKEN"))
	if token == "" {
		log.Println("⚠️ ERRORE: La variabile d'ambiente DISCORD_TOKEN non è impostata!")
	} else {
		if strings.HasPrefix(token, "Bot ") {
			token = strings.TrimPrefix(token, "Bot ")
		}

		dg, err := discordgo.New("Bot " + token)
		if err != nil {
			log.Printf("Errore durante la creazione della sessione Discord: %v", err)
		} else {
			dg.AddHandler(interactionHandler)

			err = dg.Open()
			if err != nil {
				log.Printf("Errore durante l'apertura della connessione a Discord: %v", err)
			} else {
				defer dg.Close()
				log.Println("✅ Bot 'Rome Italy 2.0' avviato con successo su Render!")
				registerCommands(dg)
			}
		}
	}

	// Mantiene il servizio in esecuzione
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	log.Println("Arresto del bot...")
}

func registerCommands(s *discordgo.Session) {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "setup",
			Description: "Configura i pannelli interattivi e i canali del server",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "server",
					Description: "Pannello generale per configurare canali (#) e ruoli (@) di ogni comando",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "apri-conto",
					Description: "Invia il pannello bancario per l'apertura del conto",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "bonifico",
					Description: "Invia il pannello per l'invio dei bonifici",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "busta-paga",
					Description: "Invia il pannello per la riscossione dello stipendio",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "cittadinanza",
					Description: "Invia il pannello anagrafe per la carta d'identità",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "porto-darmi",
					Description: "Invia il pannello per la richiesta del porto d'armi",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "immatricolazione",
					Description: "Invia il pannello PRA per immatricolare i veicoli",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "registra-arresto",
					Description: "Invia il pannello delle Forze dell'Ordine per registrare arresti",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "emetti-multa",
					Description: "Invia il pannello per emettere sanzioni stradali",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "staff-panel",
					Description: "Invia il pannello riservato alla moderazione Staff",
				},
			},
		},
	}

	for _, cmd := range commands {
		_, err := s.ApplicationCommandCreate(s.State.User.ID, "", cmd)
		if err != nil {
			log.Printf("Errore nella registrazione del comando '%s': %v", cmd.Name, err)
		}
	}
}

func interactionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		handleSlashCommands(s, i)
	case discordgo.InteractionMessageComponent:
		handleComponents(s, i)
	case discordgo.InteractionModalSubmit:
		handleModalSubmit(s, i)
	}
}

func handleSlashCommands(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		return
	}

	subCmd := options[0].Name

	switch subCmd {
	case "server":
		sendServerSetupPanel(s, i)
	case "apri-conto":
		sendApriContoPanel(s, i)
	case "busta-paga":
		sendBustaPagaPanel(s, i)
	case "cittadinanza":
		sendCittadinanzaPanel(s, i)
	case "registra-arresto":
		sendRegistraArrestoPanel(s, i)
	case "bonifico":
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "✅ Pannello `/setup bonifico` inviato con successo!",
			},
		})
	case "porto-darmi":
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "✅ Pannello `/setup porto-darmi` inviato con successo!",
			},
		})
	case "immatricolazione":
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "✅ Pannello `/setup immatricolazione` inviato con successo!",
			},
		})
	case "emetti-multa":
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "✅ Pannello `/setup emetti-multa` inviato con successo!",
			},
		})
	case "staff-panel":
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "✅ Pannello `/setup staff-panel` inviato con successo!",
			},
		})
	default:
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Sottocomando non riconosciuto.",
			},
		})
	}
}

func sendServerSetupPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "⚙️ CONFIGURAZIONE GLOBALE COMANDI - ROME ITALY 2.0",
		Description: "Seleziona dal menu a tendina il comando che vuoi configurare. Potrai scegliere i canali e i ruoli direttamente dalle liste di Discord:",
		Color:       0x0099FF,
	}

	selectMenu := discordgo.SelectMenu{
		CustomID:    "setup_server_select",
		Placeholder: "Scegli il comando da configurare...",
		Options: []discordgo.SelectMenuOption{
			{Label: "Cittadinanza & Anagrafe", Value: "cfg_cittadinanza", Description: "Imposta canali e ruoli per /cittadinanza", Emoji: &discordgo.ComponentEmoji{Name: "🇮🇹"}},
			{Label: "Porto d'Armi", Value: "cfg_porto_darmi", Description: "Imposta canali e ruoli per /porto-darmi", Emoji: &discordgo.ComponentEmoji{Name: "🔫"}},
			{Label: "Apertura Conto Bancario", Value: "cfg_apri_conto", Description: "Imposta canali e ruoli per /apri-conto", Emoji: &discordgo.ComponentEmoji{Name: "💳"}},
			{Label: "Bonifico Bancario", Value: "cfg_bonifico", Description: "Imposta canali e ruoli per /bonifico", Emoji: &discordgo.ComponentEmoji{Name: "💸"}},
			{Label: "Riscossione Busta Paga", Value: "cfg_busta_paga", Description: "Imposta canali e ruoli per /busta-paga", Emoji: &discordgo.ComponentEmoji{Name: "💵"}},
			{Label: "Immatricolazione Veicoli", Value: "cfg_immatricolazione", Description: "Imposta canali e ruoli per /immatricolazione", Emoji: &discordgo.ComponentEmoji{Name: "🚗"}},
			{Label: "Registro Arresti FDO", Value: "cfg_police", Description: "Imposta canali e ruoli per /registra-arresto", Emoji: &discordgo.ComponentEmoji{Name: "🚨"}},
			{Label: "Emissione Multe", Value: "cfg_emetti_multa", Description: "Imposta canali e ruoli per /emetti-multa", Emoji: &discordgo.ComponentEmoji{Name: "📄"}},
			{Label: "Staff & Moderazione Panel", Value: "cfg_staff_panel", Description: "Imposta canali e ruoli per /staff-panel", Emoji: &discordgo.ComponentEmoji{Name: "🛡️"}},
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

func sendApriContoPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "🏛️ APERTURA CONTO CORRENTE - ROME ITALY 2.0",
		Description: "Clicca sul bottone per scegliere il tuo istituto bancario.",
		Color:       0x2ECC71,
	}
	button := discordgo.Button{Label: "Apri Conto Corrente", Style: discordgo.SuccessButton, CustomID: "btn_apri_conto", Emoji: &discordgo.ComponentEmoji{Name: "💳"}}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{button}}}},
	})
}

func sendBustaPagaPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "💼 RISCOSSIONE BUSTA PAGA - ROME ITALY 2.0",
		Description: "Clicca sul bottone per riscattare lo stipendio.",
		Color:       0xF1C40F,
	}
	button := discordgo.Button{Label: "Riscuoti Busta Paga", Style: discordgo.PrimaryButton, CustomID: "btn_riscuoti_stipendio", Emoji: &discordgo.ComponentEmoji{Name: "💵"}}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{button}}}},
	})
}

func sendCittadinanzaPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "🇮🇹 ANAGRAFE CENTRALE - ROME ITALY 2.0",
		Description: "Richiedi la tua Carta d'Identità.",
		Color:       0x3498DB,
	}
	button := discordgo.Button{Label: "Richiedi Cittadinanza", Style: discordgo.PrimaryButton, CustomID: "btn_richiedi_cittadinanza", Emoji: &discordgo.ComponentEmoji{Name: "🆔"}}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{button}}}},
	})
}

func sendRegistraArrestoPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "🚨 BANCA DATI POLIZIA DI STATO / CARABINIERI",
		Description: "Verbale dei fermi e degli arresti.",
		Color:       0xE74C3C,
	}
	button := discordgo.Button{Label: "Registra Arresto", Style: discordgo.DangerButton, CustomID: "btn_registra_arresto", Emoji: &discordgo.ComponentEmoji{Name: "📝"}}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{button}}}},
	})
}

func handleComponents(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	// 1. Quando selezioni un comando dal menu di /setup server
	if customID == "setup_server_select" {
		selectedCategory := "cfg_cittadinanza"
		if len(i.MessageComponentData().Values) > 0 {
			selectedCategory = i.MessageComponentData().Values[0]
		}

		// Menu nativo per scegliere il canale del pannello (#)
		channelSelectPanel := discordgo.SelectMenu{
			CustomID:    "sel_chan_panel_" + selectedCategory,
			Placeholder: "Seleziona il canale del comando (#)...",
			MenuType:    discordgo.ChannelSelectMenu,
			ChannelTypes: []discordgo.ChannelType{
				discordgo.ChannelTypeGuildText,
			},
		}

		// Menu nativo per scegliere il canale dove inviare i risultati/esiti (#)
		channelSelectEsiti := discordgo.SelectMenu{
			CustomID:    "sel_chan_esiti_" + selectedCategory,
			Placeholder: "Seleziona il canale dei risultati/esiti (#)...",
			MenuType:    discordgo.ChannelSelectMenu,
			ChannelTypes: []discordgo.ChannelType{
				discordgo.ChannelTypeGuildText,
			},
		}

		// Menu nativo per scegliere il ruolo autorizzato (@)
		roleSelectAuth := discordgo.SelectMenu{
			CustomID:    "sel_role_auth_" + selectedCategory,
			Placeholder: "Seleziona il ruolo autorizzato (@)...",
			MenuType:    discordgo.RoleSelectMenu,
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("🛠️ **Configurazione per: %s**\nScegli dai menu a tendina qui sotto i canali (`#`) e il ruolo (`@`):", strings.ToUpper(selectedCategory[4:])),
				Flags:   discordgo.MessageFlagsEphemeral,
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{channelSelectPanel}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{channelSelectEsiti}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{roleSelectAuth}},
				},
			},
		})
		return
	}

	// 2. Salvataggio automatico delle scelte fatte dai menu nativi di Discord
	if strings.HasPrefix(customID, "sel_chan_panel_") || strings.HasPrefix(customID, "sel_chan_esiti_") || strings.HasPrefix(customID, "sel_role_auth_") {
		values := i.MessageComponentData().Values
		if len(values) > 0 {
			valoreScelto := values[0]
			parts := strings.Split(customID, "_")
			servizioKey := parts[len(parts)-1]

			cfg := serverConfigs[servizioKey]
			if strings.Contains(customID, "sel_chan_panel_") {
				cfg.PanelChannelID = valoreScelto
			} else if strings.Contains(customID, "sel_chan_esiti_") {
				cfg.EsitiChannelID = valoreScelto
			} else if strings.Contains(customID, "sel_role_auth_") {
				cfg.AuthRoleID = valoreScelto
			}
			serverConfigs[servizioKey] = cfg
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "✅ **Selezione salvata con successo nel sistema!**",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// 3. Gestione bottoni utenti (Banche, Cittadinanza, Arresti)
	switch customID {
	case "btn_apri_conto":
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
				{Label: "Monte dei Paschi di Siena", Value: "mps", Emoji: &discordgo.ComponentEmoji{Name: "🏰"}},
				{Label: "Credit Agricole Italia", Value: "credit_agricole", Emoji: &discordgo.ComponentEmoji{Name: "🌾"}},
				{Label: "Revolut", Value: "revolut", Emoji: &discordgo.ComponentEmoji{Name: "💳"}},
				{Label: "HYPE (Banca Sella)", Value: "hype", Emoji: &discordgo.ComponentEmoji{Name: "⚡"}},
				{Label: "N26", Value: "n26", Emoji: &discordgo.ComponentEmoji{Name: "🖤"}},
				{Label: "FinecoBank", Value: "fineco", Emoji: &discordgo.ComponentEmoji{Name: "📈"}},
				{Label: "BBVA Italia", Value: "bbva", Emoji: &discordgo.ComponentEmoji{Name: "🟡"}},
				{Label: "Buddybank", Value: "buddybank", Emoji: &discordgo.ComponentEmoji{Name: "📱"}},
			},
		}
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Scegli la banca con cui desideri aprire il conto corrente:",
				Flags:   discordgo.MessageFlagsEphemeral,
				Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{selectBanche}}},
			},
		})

	case "select_banca_choice":
		selectedBanca := "banca_generica"
		if len(i.MessageComponentData().Values) > 0 {
			selectedBanca = i.MessageComponentData().Values[0]
		}
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID: "modal_conferma_conto_" + selectedBanca,
				Title:    "Apertura Conto Corrente",
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "input_nome_cognome_rp", Label: "Nome e Cognome RP", Style: discordgo.TextInputShort, Placeholder: "Es. Mario Rossi", Required: true}}},
				},
			},
		})

	case "btn_richiedi_cittadinanza":
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID: "modal_cittadinanza",
				Title:    "Registrazione Carta d'Identita",
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "input_nome_rp", Label: "Nome RP", Style: discordgo.TextInputShort, Placeholder: "Es. Mario", Required: true}}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "input_cognome_rp", Label: "Cognome RP", Style: discordgo.TextInputShort, Placeholder: "Es. Rossi", Required: true}}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "input_data_nascita", Label: "Data di Nascita RP", Style: discordgo.TextInputShort, Placeholder: "GG/MM/AAAA", Required: true}}},
				},
			},
		})

	case "btn_registra_arresto":
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID: "modal_registra_arresto",
				Title:    "Verbale d'Arresto",
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "input_utente_arrestato", Label: "Utente Roblox Arrestato", Style: discordgo.TextInputShort, Required: true}}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "input_reati", Label: "Reati Commessi", Style: discordgo.TextInputParagraph, Placeholder: "Inseguimento...", Required: true}}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "input_prigione", Label: "Anni o Mesi di Prigione", Style: discordgo.TextInputShort, Required: true}}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "input_cauzione", Label: "Cauzione In Euro", Style: discordgo.TextInputShort, Placeholder: "5000", Required: false}}},
				},
			},
		})

	case "btn_riscuoti_stipendio":
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "✅ **Stipendio Riscosso!**", Flags: discordgo.MessageFlagsEphemeral},
		})
	}
}

func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.ModalSubmitData().CustomID

	// Invio scheda apertura conto nel canale esiti bancario scelto
	if strings.HasPrefix(customID, "modal_conferma_conto_") {
		banca := strings.TrimPrefix(customID, "modal_conferma_conto_")
		data := i.ModalSubmitData()
		nomeRP := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "🎉 **Conto Corrente Aperto con Successo!** La richiesta è stata inviata.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})

		targetChannel := serverConfigs["apri_conto"].EsitiChannelID
		if targetChannel == "" {
			targetChannel = i.ChannelID
		}

		embedEsito := &discordgo.MessageEmbed{
			Title:       "💳 NUOVA APERTURA CONTO BANCARIO",
			Color:       0x2ECC71,
			Description: fmt.Sprintf("**Utente:** <@%s>\n**Nome & Cognome RP:** %s\n**Banca:** %s", i.Member.User.ID, nomeRP, strings.ToUpper(banca)),
		}
		s.ChannelMessageSendEmbed(targetChannel, embedEsito)
		return
	}

	switch customID {
	// Invio scheda cittadinanza nel canale esiti anagrafe scelto
	case "modal_cittadinanza":
		data := i.ModalSubmitData()
		nomeRP := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
		cognomeRP := data.Components[1].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
		dataNascita := data.Components[2].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "🇮🇹 **Richiesta Cittadinanza Registrata!** Inviata nel canale di raccolta esiti.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})

		targetChannel := serverConfigs["cittadinanza"].EsitiChannelID
		if targetChannel == "" {
			targetChannel = i.ChannelID
		}

		embedAnagrafe := &discordgo.MessageEmbed{
			Title: "🇮🇹 NUOVA RICHIESTA CITTADINANZA / CARTA D'IDENTITÀ",
			Color: 0x3498DB,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Richiedente", Value: fmt.Sprintf("<@%s>", i.Member.User.ID), Inline: true},
				{Name: "Nome RP", Value: nomeRP, Inline: true},
				{Name: "Cognome RP", Value: cognomeRP, Inline: true},
				{Name: "Data di Nascita", Value: dataNascita, Inline: true},
			},
		}
		s.ChannelMessageSendEmbed(targetChannel, embedAnagrafe)

	// Invio verbale arresto nel canale esiti registro FDO scelto
	case "modal_registra_arresto":
		data := i.ModalSubmitData()
		utenteArrestato := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
		reati := data.Components[1].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
		prigione := data.Components[2].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
		cauzione := data.Components[3].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "🚨 **Verbale Registrato!** Inviato nel canale del registro ufficiale.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})

		targetChannel := serverConfigs["police"].EsitiChannelID
		if targetChannel == "" {
			targetChannel = i.ChannelID
		}

		embedArresto := &discordgo.MessageEmbed{
			Title: "🚨 VERBALE D'ARRESTO UFFICIALE",
			Color: 0xE74C3C,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Agente", Value: fmt.Sprintf("<@%s>", i.Member.User.ID), Inline: true},
				{Name: "Arrestato", Value: utenteArrestato, Inline: true},
				{Name: "Pena", Value: prigione, Inline: true},
				{Name: "Cauzione", Value: cauzione, Inline: true},
				{Name: "Reati", Value: reati, Inline: false},
			},
		}
		s.ChannelMessageSendEmbed(targetChannel, embedArresto)
	}
}
