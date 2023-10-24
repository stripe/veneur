pipeline {
    agent {
        label 'amazon-linux2'
    }

    parameters {
        string(name: "BRANCH_REV", defaultValue: "v14.2.0-prw-sink", description: "Revision to build")
        string(name: "IMAGE_TAG", defaultValue: "v14.2.0-3", description: "Image tag")
        booleanParam(name: 'DRY_RUN', defaultValue: true, description: 'Perform a dry run (does not push images)')
        booleanParam(name: 'PUSH_TO_QUAY', defaultValue: false, description: 'Push to Quay as well as Artifactory')
    }

    environment {
        ARTIFACTORY_REPO="docker.internal.sysdig.com"
        IMAGE_NAME_PREFIX = "${env.ARTIFACTORY_REPO}/docker/veneur-rw"
        IMAGE_NAME = "${env.IMAGE_NAME_PREFIX}:${params.IMAGE_TAG}"
        IMAGE_NAME_ALPINE = "${env.IMAGE_NAME_PREFIX}:${params.IMAGE_TAG}-alpine3.14"

        QUAY_PREFIX = "quay.io/sysdig/veneur_sink"
        QUAY_IMAGE_NAME = "${env.QUAY_PREFIX}:${params.IMAGE_TAG}"
        QUAY_IMAGE_NAME_ALPINE = "${env.QUAY_PREFIX}:${params.IMAGE_TAG}-alpine3.14"
    }

    stages {

        stage('Build') {
            steps {
                script {
                    sh "docker build -f public-docker-images/Dockerfile-debian-sid --build-arg VERSION=${env.GIT_COMMIT} --build-arg BUILD_REF=${params.BRANCH_REV} -t ${env.IMAGE_NAME} --no-cache ."
                    sh "docker build -f public-docker-images/Dockerfile-alpine --build-arg VERSION=${env.GIT_COMMIT} --build-arg BUILD_REF=${params.BRANCH_REV} -t ${env.IMAGE_NAME_ALPINE} --no-cache ."
                }
            }
        }

        stage('Publish image to Artifactory') {
            steps {
                script {
                    withCredentials([usernamePassword(credentialsId: 'jenkins-artifactory', usernameVariable: 'ARTIFACTORY_USERNAME', passwordVariable:'ARTIFACTORY_PASSWORD')]) {
                        sh "docker login -u=$ARTIFACTORY_USERNAME -p=$ARTIFACTORY_PASSWORD ${env.ARTIFACTORY_REPO}"
                    }
                    if (params.DRY_RUN) {
                        echo "docker push ${env.IMAGE_NAME}"
                        echo "docker push ${env.IMAGE_NAME_ALPINE}"
                    } else {
                        sh "docker push ${env.IMAGE_NAME}"
                        sh "docker push ${env.IMAGE_NAME_ALPINE}"
                    }
                }
            }
        }

        stage('Publish image to Quay') {
            steps {
                script {
                    withCredentials([usernamePassword(credentialsId: 'quay-robot-account', usernameVariable: 'QUAY_USERNAME', passwordVariable:'QUAY_PASSWORD')]) {
                        sh "docker login -u=${QUAY_USERNAME} -p=${QUAY_PASSWORD} quay.io"
                    }
                    if (params.DRY_RUN || !params.PUSH_TO_QUAY) {
                        echo "docker push ${env.QUAY_IMAGE_NAME}"
                        echo "docker push ${env.QUAY_IMAGE_NAME_ALPINE}"
                    } else {
                        sh "docker tag ${env.IMAGE_NAME} ${env.QUAY_IMAGE_NAME}"
                        sh "docker push ${env.QUAY_IMAGE_NAME}"
                        sh "docker tag ${env.IMAGE_NAME_ALPINE} ${env.QUAY_IMAGE_NAME_ALPINE}"
                        sh "docker push ${env.QUAY_IMAGE_NAME_ALPINE}"
                    }
                }
            }
        }
    }

    post {
        success {
            echo 'All done.'
        }
        cleanup {
            echo 'Cleaning up...'
            script {
               sh "docker image rm ${env.IMAGE_NAME}"
               sh "docker image rm ${env.IMAGE_NAME_ALPINE}"
            }
            cleanWs()
        }
    }
}
