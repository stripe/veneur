Contributing to Veneur
=================

Thanks for contributing to Veneur!

This is a short document letting you know what you can expect when submitting a patch.

### Submitting a Change or Feature Request

* If you want to being work on a major change or feature addition, please create an issue first, so we can discuss it! For smaller changes, there's no need to create a ticket first.
* When submitting a change, it's helpful to know if you have tested it in production on your own systems, and at what scale. Or if you haven't tested it at scale yet, we'll do that as part of the review process.

### Code Review

All pull requests will be reviewed by someone on Stripe's Observability team. At present, those users are:

* aditya-stripe (aka chimeracoder)
* an-stripe
* asf-stripe
* cory-stripe (aka gphat)
* joshu-stripe
* krisreeves-stripe

There's no need to pick a reviewer; if you submit a pull request, we'll see it and figure out who should review it.

If your pull request involves large or sensitive changes, it will probably need to be reviewed by either Aditya Mukerjee (chimeracoder || aditya-stripe) or Cory Watson (cory-stripe || gphat).

For larger changes, there may be a delay in reviewing/merging if Aditya or Cory is unavailable, or if we need to complete merge some other related work first as a prerequisite. If you have any questions along the way, though, feel free to ask on the thread, and we'll be happy to help out.

### Help Us Help You!

We highly recommend [allowing edits from maintainers](https://help.github.com/articles/allowing-changes-to-a-pull-request-branch-created-from-a-fork/) (ie, us) to your branch. Since Veneur is actively developed, there's a good chance that other pull requests will have been merged in between when you submit a pull request and when we approve it. Allowing us to edit your branch means that we can rebase and fix any merge conflicts for you.

### When Will Your Pull Request Get Merged?

In addition to the code review process, there may be operational concerns before we can merge a pull request. For example, a change may need extensive testing or profiling against live data before we can be confident that it will handle the scale of our production volume. Depending on the change, this may take some time, but we'll keep you updated throughout the process.
