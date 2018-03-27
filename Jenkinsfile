#!/usr/bin/env groovy

pipeline {
  agent {
    label "rhel7"
  }
  options {
        ansiColor('xterm')
        timestamps()
        timeout(time: 60, unit: "MINUTES")
  }
  stages {
    stage("build") {
      steps {
        sh '''#!/usr/bin/env bash
          export BASE_OS=rhel7
          export GIT_COMMIT=$(git rev-parse HEAD)
          export GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
          export BUILD_VERSION=$(build-tools/version-tool version)
          export BUILD_INFO=$(build-tools/version-tool build-info)
          BASE_PUSH_TARGET="$DOCKER_NAMESPACE/f5-ipam-ctlr"
          export IMG_TAG="${BASE_PUSH_TARGET}:${GIT_COMMIT}-$BASE_OS"
          export BUILD_IMG_TAG="${BASE_PUSH_TARGET}-devel:${GIT_COMMIT}-$BASE_OS"
          export CLEAN_BUILD=true
          make prod
          docker tag "$IMG_TAG" "$BASE_PUSH_TARGET:devel-$GIT_BRANCH-$BASE_OS"
          docker push "$IMG_TAG"
          docker push "$BASE_PUSH_TARGET:devel-$GIT_BRANCH-$BASE_OS"
        '''
      }
    }
  }
  post {
    always {
      // cleanup workspace
      dir("${env.WORKSPACE}") { deleteDir() }
    }
  }
}
