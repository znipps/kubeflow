# Rebasing upstream

Configure the Kubeflow repository as the remote upstream:

```shell
git remote add upstream git@github.com:kubeflow/kubeflow.git
git remote set-url --push upstream DISABLE
```

Fetch the latest changes in the Kubeflow repository:

```shell
git fetch upstream -p
```

Rebase and favor local changes upon conflicts:

```shell
git rebase -Xtheirs upstream/main
```

*The above may cause conflicts. Learn how to fix these conflicts by reading the official [git-scm docs](https://git-scm.com/docs/git-rebase).*

Finally, push these changes to the ODH repository:

```shell
git push origin --force
```
