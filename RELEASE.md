# CruiseKube Release Process

This document outlines the release process for CruiseKube.

## Release Workflow Overview

We have 1 release type:
1. **Stable Release**: Manually triggered via Github releases. 


## 1. Stable Release

Stable releases require manual preparation and are triggered by creating a GitHub release.

### Preparation Steps

1. Update version information:
   
   - Update `charts/cruisekube/Chart.yaml` with the new version number:
     ```yaml
     version: X.Y.Z
     appVersion: "X.Y.Z"
     ```
   
   - Update `charts/cruisekube/values.yaml` to reference the specific commit SHA:
     ```yaml
     cruisekubeController:
       image:
         tag: vX.Y.Z
     cruisekubeWebhook:
       image:
         tag: vX.Y.Z
     cruisekubeFrontend:
       image:
         tag: vX.Y.Z
     ```

2. Update `CHANGELOG.md` with the new version number:
   ```markdown
   ## vX.Y.Z (YYYY-MM-DD)
   ### New
   - <Where the change was done>: <What was added>
   - <Resolved>: ...
   - <General>: ...

   ### Experimental
   - <Where the change was done>: <What was added>

   ### Improvements
   - <Where the change was done>: <What was improved>

   ### Fixes
   - <Where the change was done>: <What was fixed>

   ### Breaking Changes
   - <Where the change was done>: <What was changed>

   ### Other
   - <Where the change was done>: <What was changed>

   ### New Contributors
   - @rethil made their first contribution in #154
   etc...
   ```

3. Create a pull request with these changes
4. Review and merge the PR to `main`

### Release Steps

1. Create a new GitHub release:
   - Tag format: `vX.Y.Z`
   - Title: `CruiseKube vX.Y.Z`
   - Include release notes detailing changes
        ```markdown
        We are happy to release CruiseKube vX.Y.Z 🎉

        Here are some highlights of this release:
        - Highlight 1
        - Highlight 2

        Here are the breaking changes of this release:
        - Breaking Change 1
        - Breaking Change 2

        Learn how to deploy CruiseKube by reading [our documentation](https://CruiseKube.dev).

        # New
        - <Where the change was done>: <What was added>
        - <Resolved>: ...
        - <General>: ...

        ## Experimental
        - <Where the change was done>: <What was added>

        # Improvements
        - <Where the change was done>: <What was improved>

        # Fixes
        - <Where the change was done>: <What was fixed>

        # Breaking Changes
        - <Where the change was done>: <What was changed>

        # Other
        - <Where the change was done>: <What was changed>

        # New Contributors
        - @rethil made their first contribution in #154
        etc...
        ```
    - Feel free to add more sections as needed, or remove what is not needed.


2. The `.github/workflows/release.yaml` workflow is triggered:
   - Helm chart is packaged
   - Chart is pushed to the JFrog Artifactory Helm repository


## Version Numbering

CruiseKube follows semantic versioning (SemVer):
- **X**: Major version for incompatible API changes
- **Y**: Minor version for new functionality in a backward-compatible manner
- **Z**: Patch version for backward-compatible bug fixes


## Rollback Procedure

If issues are discovered in a release:
1. For critical issues, create a hotfix release
2. For stable releases.
   1. Prepare a new patch release with fixes.
   2. Mark the old release as deprecated or bad.


