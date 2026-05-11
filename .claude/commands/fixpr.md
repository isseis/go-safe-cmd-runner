## 準備

現在のブランチに対応する PR を確認する。

```
gh pr view --json number,url,headRefName
```

PR が存在しない場合は終了する。
owner・repo・PR 番号を後続のステップで使うために控えておく。

## 未解決コメントの取得

GraphQL で未解決レビュースレッドの一覧を取得する。

```
gh api graphql -F owner=OWNER -F repo=REPO -F number=NUMBER -f query='
  query($owner:String!, $repo:String!, $number:Int!) {
    repository(owner:$owner, name:$repo) {
      pullRequest(number:$number) {
        reviewThreads(first:100) {
          nodes {
            id
            isResolved
            comments(first:10) {
              nodes {
                id
                databaseId
                body
                path
                line
                author { login }
                url
              }
            }
          }
        }
      }
    }
  }
'
```

`isResolved: false` のスレッドのみを対象にする。対象がなければ終了する。

## 各未解決スレッドへの対応

各スレッドに対して順番に以下を実施する。

### 対応が明らかな場合

1. 指摘に従いコードを修正する。
2. `make lint` と `make test` を実行し、エラーがないことを確認する。
3. commit する。
4. 対応内容を PR コメントとしてリプライする（英語で記述する）。

   ```
   gh api repos/OWNER/REPO/pulls/NUMBER/comments/COMMENT_ID/replies \
     -X POST -f body="Description of the fix in English"
   ```

5. スレッドを resolved にする。

   ```
   gh api graphql -F threadId=THREAD_ID -f query='
     mutation($threadId:ID!) {
       resolveReviewThread(input:{threadId:$threadId}) {
         thread { id isResolved }
       }
     }
   '
   ```

### 対応方針が明らかでない場合

スキップして次のスレッドへ進む（後のステップで再検討）。

## push

明らかなコメントへの対応が全て完了したら `git push` する。

## 未解決スレッドの再検討

スキップしたスレッドについて、それぞれ以下を提示する。

- **問題の要約**: コメントが指摘している問題を簡潔にまとめる。
- **対応案**: 考えられる選択肢を複数挙げ、それぞれ pros と cons を示す。
- **推奨案**: 可能であれば一つ選び、理由を述べる。
