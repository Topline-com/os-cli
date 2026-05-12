# Examples

## Qualified pipeline activity this week

```bash
topline --agent pipeline audit \
  --pipeline-id CLUy1QapsrEeBiNrmQiL \
  --since 2026-05-11 \
  --status open
```

## Search open opportunities

```bash
topline opportunities search \
  --pipeline-id CLUy1QapsrEeBiNrmQiL \
  --status open \
  --limit 100
```

## Pull recent messages

```bash
topline conversations messages \
  --conversation-id qwuPtEkodCM8k232NsWg \
  --limit 10 \
  --mask-pii
```

## Create a task

```bash
topline tasks create \
  --contact-id CONTACT_ID \
  --title "Follow up" \
  --due-date 2026-05-13T16:00:00-04:00
```

## Initialize local mirror DB

```bash
topline sync init --db topline.db
```
