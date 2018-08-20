package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	pocket "github.com/brimstone/go-pocket"
	"github.com/fsnotify/fsnotify"
	"github.com/google/go-github/github"
	mastodon "github.com/mattn/go-mastodon"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
)

func getStarredRepos(client *github.Client) ([]github.Repository, error) {
	opt := &github.ActivityListStarredOptions{
		ListOptions: github.ListOptions{PerPage: 25},
	}
	var allRepos []github.Repository
	var repos, _, err = client.Activity.ListStarred(context.Background(), "", opt)
	if err != nil {
		return []github.Repository{}, err
	}
	for _, repo := range repos {
		allRepos = append(allRepos, *repo.Repository)
	}
	return allRepos, nil
}

func getToots(c *mastodon.Client) ([]*mastodon.Status, error) {
	me, err := c.GetAccountCurrentUser(context.Background())
	if err != nil {
		return nil, err
	}

	return c.GetAccountStatuses(context.Background(), me.ID, nil)
}

func logit(logger *log.Logger, level int, format string, args ...interface{}) {
	if viper.GetInt("loglevel") >= level {
		logger.Printf(format, args...)
	}
}

func checkStars(clientMasto *mastodon.Client, toots []*mastodon.Status) error {
	logger := log.New(os.Stderr, "[checkStars] ", log.LstdFlags)
	// Get Starred Repos
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: viper.GetString("github.token")},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	client := github.NewClient(tc)
	repos, err := getStarredRepos(client)
	if err != nil {
		return err
	}

	// Check each repo
	for _, repo := range repos {
		repourl := *repo.HTMLURL
		// if the repo is found in list of toots, exit
		found := false
		for _, toot := range toots {
			if toot.Reblog != nil {
				continue
			}
			if toot.InReplyToID != nil {
				continue
			}
			if strings.Contains(toot.Content, repourl) {
				found = true
				break
			}
		}
		if found {
			return nil
		}
		logger.Printf("Checking %s\n", repourl)

		statustext := "I just starred " + repourl + "\n\n"
		if *repo.Description != "" {
			statustext += *repo.Description + "\n\n"
		}
		if repo.License != nil {
			statustext += " #" + *repo.License.Key
		}
		if repo.Language != nil {
			statustext += " #" + *repo.Language
		}
		for _, topic := range repo.Topics {
			statustext += " #" + topic
		}
		statustext = strings.Replace(statustext, "\n ", "\n", -1)
		// otherise, toot time!
		_, err = clientMasto.PostStatus(
			context.Background(),
			&mastodon.Toot{
				Status: statustext,
			},
		)
		if err != nil {
			return err
		}
	}
	return nil
}
func checkArticles(clientMasto *mastodon.Client, toots []*mastodon.Status) error {
	logger := log.New(os.Stderr, "[checkArticles] ", log.LstdFlags)

	oldestArticle := toots[len(toots)-1].CreatedAt
	found := false

	for _, toot := range toots {
		if toot.Reblog != nil {
			continue
		}
		if toot.InReplyToID != nil {
			continue
		}
		if strings.Contains(toot.Content, "I just pocketed:") {
			logit(logger, 2, "Found pocketed article")
			oldestArticle = toot.CreatedAt
			found = true
			break
		}
	}
	logit(logger, 2, "Oldest Article: %s", oldestArticle)

	p := pocket.NewPocketClient(&pocket.PocketClientOptions{
		ConsumerKey: viper.GetString("pocket.key"),
		AccessToken: viper.GetString("pocket.token"),
	})

	articles, err := p.Get(&pocket.GetOptions{
		DetailType: pocket.GetDetailTypeComplete,
		Since:      oldestArticle,
		State:      pocket.GetStateArchive,
	})
	if err != nil {
		return err
	}

	for _, article := range articles.List {
		logit(logger, 1, article.ResolvedTitle)
		statustext := "I just pocketed: " + article.ResolvedTitle + "\n\n"
		statustext += article.GivenURL + "\n\n"
		for _, tag := range article.Tags {
			statustext += " #" + strings.Replace(tag.Tag, " ", "", -1)
		}
		statustext = strings.Replace(statustext, "\n ", "\n", -1)
		_, err = clientMasto.PostStatus(
			context.Background(),
			&mastodon.Toot{
				Status: statustext,
			},
		)
		if err != nil {
			return err
		}
		// If we didn't find a previous pocketed toot, limit ourselves to just
		// this one
		if !found {
			break
		}
	}
	return nil
}

func checkAll(c *mastodon.Client) error {
	logger := log.New(os.Stderr, "[checkAll] ", log.LstdFlags)
	logit(logger, 1, "Getting toots")
	toots, err := getToots(c)
	if err != nil {
		return err
	}

	err = checkStars(c, toots)
	if err != nil {
		return err
	}
	err = checkArticles(c, toots)
	if err != nil {
		return err
	}
	logit(logger, 1, "Finished checking")
	return nil
}

func main() {
	logger := log.New(os.Stderr, "[main] ", log.LstdFlags)
	flag.String("github.token", "", "github token")
	flag.String("github.username", "", "github username")
	flag.String("mastodon.client-id", "", "mastodon id")
	flag.String("mastodon.client-secret", "", "mastodon secret")
	flag.String("mastodon.username", "", "mastodon username")
	flag.String("mastodon.password", "", "mastodon password")
	flag.String("frequency", "", "Check frequency")
	configpath := flag.String("config", "", "Config file path")
	flag.Parse()
	if *configpath != "" {
		viper.SetConfigName(filepath.Base(*configpath))
		viper.AddConfigPath(filepath.Dir(*configpath))
	} else {
		// setup configs
		viper.SetConfigName("config")
		viper.AddConfigPath("/")
		viper.AddConfigPath("$HOME/.pocket")
		viper.AddConfigPath(".")
	}
	err := viper.ReadInConfig()
	if err != nil {
		logger.Println(err)
	}
	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		logit(logger, 2, "Config file changed:", e.Name)
	})
	viper.AutomaticEnv()
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)

	//logger.Printf("username: %s\n", viper.GetString("mastodon.username"))
	// Get Toots
	if viper.GetString("mastodon.client-id") == "" {
		logger.Fatal("--mastodon.client-id or MASTODON.CLIENT-ID must be set")
	}
	if viper.GetString("mastodon.client-secret") == "" {
		logger.Fatal("--mastodon.client-secret or MASTODON.CLIENT-SECRET must be set")
	}
	if viper.GetString("mastodon.username") == "" {
		logger.Fatal("--mastodon.username or MASTODON.USERNAME must be set")
	}
	if viper.GetString("mastodon.password") == "" {
		logger.Fatal("--mastodon.password or MASTODON.PASSWORD must be set")
	}
	c := mastodon.NewClient(&mastodon.Config{
		Server:       "https://mastodon.social",
		ClientID:     viper.GetString("mastodon.client-id"),
		ClientSecret: viper.GetString("mastodon.client-secret"),
	})
	err = c.Authenticate(context.Background(), viper.GetString("mastodon.username"), viper.GetString("mastodon.password"))
	if err != nil {
		logger.Fatal(err)
	}

	err = checkAll(c)
	if err != nil {
		logger.Fatal(err)
	}
	if viper.GetString("frequency") == "" {
		return
	}
	// If we're suppose to loop
	dur, err := time.ParseDuration(viper.GetString("frequency"))
	if err != nil {
		logger.Fatal(err)
	}
	ticker := time.Tick(dur)
	for range ticker {
		err = checkAll(c)
		if err != nil {
			logger.Println(err)
		}
	}
}
