# jellyfin-discord
<h3>Display what you're watching on Jellyfin on Discord.</h3>
Example:
<br>
<img width="363" height="151" alt="image" src="https://github.com/user-attachments/assets/8bd15a56-ad94-4f41-8634-f174446d8796" />
<br>
<h3>Configuration</h3>
Use `config.json` to configure the program. Copy it to a local file before adding tokens or API keys.
<p>An example configuration file could be:
<br>
<img width="531" height="288" alt="image" src="https://github.com/user-attachments/assets/2b12a876-c14b-4263-9061-380f45c3bcd4" />
</p>

<h4>Configuration</h4>
<p>The available values are: jellyfin_url, jellyfin_token, tmdb_api_key, omdb_api_key, discord_app_id, target_user, poll_interval, show_paused, episode_thumbnails, fallback_artwork, generic_item_text, anime_tags, anilist_enabled, enable_buttons, public_jellyfin_url, and disable_music.</p>

<ul>
  <li><b>episode_thumbnails</b> (bool): fetch episode-specific stills from TMDB instead of the series poster</li>
  <li><b>fallback_artwork</b> (bool): use Jellyfin's own artwork endpoint if TMDB has no poster</li>
  <li><b>public_jellyfin_url</b> (string): public URL used when Discord needs to load Jellyfin artwork or links</li>
</ul>

<h3>Build</h3>
<pre><code>make test
make build
make build-linux
make build-windows</code></pre>

<h3>Features</h3>
<ul>
  <li><b>Config validation</b>: On startup, required fields (jellyfin_url, jellyfin_token, discord_app_id, target_user) are validated. If any are missing, an error is logged and the program exits.</li>
  <li><b>Auto-reconnect</b>: If Jellyfin becomes unreachable, the program logs a warning and retries on the next poll interval instead of crashing. Configuration reload will also reconnect to Discord if the App ID changes.</li>
  <li><b>Music support</b>: Music tracks (type "Audio") are now supported. Discord will show the track name, artist, and album.</li>
  <li><b>Config reload</b>: Send a SIGHUP signal to reload config.json at runtime without restarting. Use: <code>kill -HUP &lt;pid&gt;</code></li>
</ul>
