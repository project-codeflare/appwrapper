We use Jekyll to generate static html that can be served as a GitHub
page for the project.

The GitHub action
[jekyll-gh-pages](../.github/workflows/jekyll-gh-pages.yaml) runs whenever
a change to the `_site` directory is pushed to the main branch.

To host the website locally, you need a a Ruby 3.1 environment.  Then in
the [site](../site) directory do `bundle install` followed by
`bundle exec jekyll serve`.
