## Requirements

1. Telegram token stored in *BOT_TG_TOKEN* environment variable
2. MongoDB instance. Connection data should be stored in the following variables:
- *BOT_DB_HOST* — MongoDB host, for example "localhost:27017"
- *BOT_DB_NAME* — database name
- *BOT_DB_USERNAME* — database user name
- *BOT_DB_PASSWORD* — database user password

## Future features

1. Split a bill item in the proportion
    > /add 100 @cat @dog  1:2

2. Set a different accountant for a bill item
    > /add 50 @rabbit > @fox

3. Hashtags
    > /add 200 @mouse #cheese
    >
    > remove #cheese

## Attribution

This bot was inspired by [Billy the Splitter](http://telegram.me/billssplitterbot) bot
